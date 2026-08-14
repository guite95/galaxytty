package scrcpy

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/galaxytty/galaxytty/internal/domain"
)

type Config struct {
	Path, Target, Package string
	Width, Height         int
	StartupTimeout        time.Duration
	InputMode             domain.TextInputMode
	KeepActive            bool
	VideoPlayback         bool
}

type Manager struct {
	config Config

	mu             sync.Mutex
	session        *displaySession
	starting       chan struct{}
	startingCancel context.CancelFunc

	lookupPath    func(string) (string, error)
	createTemp    func() (*os.File, error)
	startProcess  func(context.Context, string, []string) (process, error)
	syncClipboard func(context.Context, int) error
	stopTimeout   time.Duration
}

const defaultStopTimeout = 2 * time.Second

type displaySession struct {
	process    process
	display    domain.VirtualDisplay
	done       chan struct{}
	recordPath string
}

type process interface {
	LogReader() io.Reader
	PID() int
	Wait() error
	Signal(os.Signal) error
	Kill() error
}

type commandProcess struct {
	cmd    *exec.Cmd
	logs   *io.PipeReader
	writer *io.PipeWriter
}

func (p *commandProcess) LogReader() io.Reader { return p.logs }
func (p *commandProcess) PID() int             { return p.cmd.Process.Pid }

func (p *commandProcess) Wait() error {
	err := p.cmd.Wait()
	_ = p.writer.Close()
	return err
}

func (p *commandProcess) Signal(signal os.Signal) error { return p.cmd.Process.Signal(signal) }
func (p *commandProcess) Kill() error                   { return p.cmd.Process.Kill() }

func BuildArgs(cfg Config, recordPath string) []string {
	args := []string{
		"-s", cfg.Target,
		fmt.Sprintf("--new-display=%dx%d", cfg.Width, cfg.Height),
		"--display-ime-policy=local",
		"--start-app=" + cfg.Package,
		"--record=" + recordPath,
		"--no-audio",
	}
	if cfg.KeepActive {
		args = append(args, "--keep-active")
	}
	if cfg.InputMode == domain.TextInputClipboard {
		return append(args,
			"--window-title=GalaxyTTY-"+filepath.Base(recordPath),
			"--window-width=1",
			"--window-height=1",
			"--window-borderless",
		)
	}
	if !cfg.VideoPlayback {
		args = append(args, "--no-video-playback")
	}
	return args
}

func NewManager(cfg Config) (*Manager, error) {
	if cfg.InputMode == "" {
		cfg.InputMode = domain.TextInputIntentBody
	}
	if cfg.Target == "" || cfg.Package == "" || cfg.Width <= 0 || cfg.Height <= 0 || cfg.StartupTimeout <= 0 || !cfg.InputMode.Valid() {
		return nil, fmt.Errorf("invalid scrcpy manager configuration")
	}
	if cfg.Path == "" {
		cfg.Path = "scrcpy"
	}
	return &Manager{
		config:        cfg,
		lookupPath:    exec.LookPath,
		createTemp:    createRecordFile,
		startProcess:  startCommand,
		syncClipboard: syncClipboardShortcut,
		stopTimeout:   defaultStopTimeout,
	}, nil
}

func (m *Manager) SyncClipboard(ctx context.Context) error {
	m.mu.Lock()
	session := m.session
	m.mu.Unlock()
	if session == nil {
		return fmt.Errorf("%w", domain.ErrClipboardSync)
	}
	select {
	case <-session.done:
		return fmt.Errorf("%w: %w", domain.ErrClipboardSync, domain.ErrScrcpyExited)
	default:
	}
	if err := m.syncClipboard(ctx, session.process.PID()); err != nil {
		return fmt.Errorf("%w: %w", domain.ErrClipboardSync, err)
	}
	return nil
}

func (m *Manager) Start(ctx context.Context) (domain.VirtualDisplay, error) {
	for {
		m.mu.Lock()
		if session := m.session; session != nil {
			select {
			case <-session.done:
				m.removeRecord(session.recordPath)
				m.session = nil
			default:
				display := session.display
				m.mu.Unlock()
				return display, nil
			}
		}
		if starting := m.starting; starting != nil {
			m.mu.Unlock()
			select {
			case <-starting:
				continue
			case <-ctx.Done():
				return domain.VirtualDisplay{}, startupError(ctx.Err())
			}
		}

		startCtx, cancel := context.WithCancel(ctx)
		starting := make(chan struct{})
		m.starting = starting
		m.startingCancel = cancel
		m.mu.Unlock()

		session, err := m.startSession(startCtx)

		m.mu.Lock()
		cancelled := startCtx.Err() != nil
		if err == nil && !cancelled {
			m.session = session
		}
		m.starting = nil
		m.startingCancel = nil
		close(starting)
		m.mu.Unlock()
		cancel()

		if err != nil {
			return domain.VirtualDisplay{}, err
		}
		if cancelled {
			_ = terminateAndReap(context.Background(), session.process, session.done, m.stopTimeout)
			m.removeRecord(session.recordPath)
			return domain.VirtualDisplay{}, startupError(context.Canceled)
		}
		return session.display, nil
	}
}

func (m *Manager) Stop(ctx context.Context) error {
	m.mu.Lock()
	session := m.session
	m.session = nil
	cancel := m.startingCancel
	starting := m.starting
	m.mu.Unlock()

	if cancel != nil {
		cancel()
		if starting != nil {
			select {
			case <-starting:
			case <-ctx.Done():
				return startupError(ctx.Err())
			}
		}
	}
	if session == nil {
		return nil
	}
	defer m.removeRecord(session.recordPath)
	return terminateAndReap(ctx, session.process, session.done, m.stopTimeout)
}

func (m *Manager) Healthy(context.Context) bool {
	m.mu.Lock()
	session := m.session
	m.mu.Unlock()
	if session == nil {
		return false
	}
	select {
	case <-session.done:
		return false
	default:
		return true
	}
}

func (m *Manager) startSession(ctx context.Context) (*displaySession, error) {
	path, err := m.lookupPath(m.config.Path)
	if err != nil {
		return nil, fmt.Errorf("%w", domain.ErrScrcpyNotFound)
	}

	record, err := m.createTemp()
	if err != nil {
		return nil, fmt.Errorf("%w", domain.ErrScrcpyStartup)
	}
	recordPath := record.Name()
	if err := record.Close(); err != nil {
		m.removeRecord(recordPath)
		return nil, fmt.Errorf("%w", domain.ErrScrcpyStartup)
	}

	proc, err := m.startProcess(ctx, path, BuildArgs(m.config, recordPath))
	if err != nil {
		m.removeRecord(recordPath)
		return nil, fmt.Errorf("%w", domain.ErrScrcpyStartup)
	}
	done := make(chan struct{})
	go func() {
		_ = proc.Wait()
		close(done)
	}()

	startupCtx, cancel := context.WithTimeout(ctx, m.config.StartupTimeout)
	defer cancel()
	id, err := waitForDisplayID(startupCtx, proc.LogReader(), done)
	if err != nil {
		_ = terminateAndReap(startupCtx, proc, done, m.stopTimeout)
		m.removeRecord(recordPath)
		return nil, err
	}
	return &displaySession{
		process: proc,
		display: domain.VirtualDisplay{
			AndroidDisplayID: id,
			Width:            m.config.Width,
			Height:           m.config.Height,
		},
		done:       done,
		recordPath: recordPath,
	}, nil
}

func waitForDisplayID(ctx context.Context, logs io.Reader, done <-chan struct{}) (int64, error) {
	ids := make(chan int64, 1)
	go func() {
		scanner := bufio.NewScanner(logs)
		scanner.Buffer(make([]byte, 1024), 1024*1024)
		for scanner.Scan() {
			if id, err := ParseDisplayID(scanner.Text()); err == nil {
				select {
				case ids <- id:
				default:
				}
			}
		}
	}()

	select {
	case id := <-ids:
		return id, nil
	case <-done:
		return 0, fmt.Errorf("%w: %w", domain.ErrScrcpyStartup, domain.ErrScrcpyExited)
	case <-ctx.Done():
		return 0, startupError(ctx.Err())
	}
}

func terminateAndReap(ctx context.Context, proc process, done <-chan struct{}, timeout time.Duration) error {
	select {
	case <-done:
		return nil
	default:
	}
	if timeout <= 0 {
		timeout = defaultStopTimeout
	}
	graceCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := proc.Signal(os.Interrupt); err != nil {
		return killAndReap(proc, done, timeout)
	}
	select {
	case <-done:
		return nil
	case <-graceCtx.Done():
		return killAndReap(proc, done, timeout)
	}
}

func killAndReap(proc process, done <-chan struct{}, timeout time.Duration) error {
	select {
	case <-done:
		return nil
	default:
	}
	if err := proc.Kill(); err != nil {
		select {
		case <-done:
			return nil
		default:
			return fmt.Errorf("%w", domain.ErrScrcpyExited)
		}
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
		return nil
	case <-timer.C:
		return fmt.Errorf("%w", domain.ErrScrcpyExited)
	}
}

func startupError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%w: %w", domain.ErrScrcpyStartup, domain.ErrVirtualDisplayIDNotFound)
	}
	if errors.Is(err, context.Canceled) {
		return fmt.Errorf("%w: %w", domain.ErrScrcpyStartup, context.Canceled)
	}
	return fmt.Errorf("%w", domain.ErrScrcpyStartup)
}

func (m *Manager) removeRecord(path string) {
	_ = os.Remove(path)
}

func createRecordFile() (*os.File, error) {
	return os.CreateTemp("", "galaxytty-scrcpy-*.mp4")
}

func startCommand(_ context.Context, path string, args []string) (process, error) {
	reader, writer := io.Pipe()
	cmd := exec.Command(path, args...)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Start(); err != nil {
		_ = reader.Close()
		_ = writer.Close()
		return nil, err
	}
	return &commandProcess{cmd: cmd, logs: reader, writer: writer}, nil
}

func syncClipboardShortcut(ctx context.Context, pid int) error {
	return exec.CommandContext(ctx, "osascript", "-e", clipboardShortcutScript(pid)).Run()
}

func clipboardShortcutScript(pid int) string {
	return `tell application "System Events"
set previousProcess to first application process whose frontmost is true
set targetProcess to first application process whose unix id is ` + strconv.Itoa(pid) + `
set frontmost of targetProcess to true
delay 0.05
key code 9 using command down
delay 0.05
try
set frontmost of previousProcess to true
end try
end tell`
}

var _ domain.VirtualDisplayManager = (*Manager)(nil)
