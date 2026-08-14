package scrcpy

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/galaxytty/galaxytty/internal/domain"
)

func TestBuildArgsSeparatesIntentAndClipboardModes(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode domain.TextInputMode
		want []string
	}{
		{
			name: "intent body headless",
			mode: domain.TextInputIntentBody,
			want: []string{
				"-s", "synthetic-target",
				"--new-display=1080x1920",
				"--display-ime-policy=local",
				"--start-app=com.samsung.android.messaging",
				"--record=/synthetic/record.mp4",
				"--no-audio",
				"--no-video-playback",
			},
		},
		{
			name: "clipboard compatibility window",
			mode: domain.TextInputClipboard,
			want: []string{
				"-s", "synthetic-target",
				"--new-display=1080x1920",
				"--display-ime-policy=local",
				"--start-app=com.samsung.android.messaging",
				"--record=/synthetic/record.mp4",
				"--no-audio",
				"--window-title=GalaxyTTY-record.mp4",
				"--window-width=1",
				"--window-height=1",
				"--window-borderless",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Config{
				Target: "synthetic-target", Package: "com.samsung.android.messaging",
				Width: 1080, Height: 1920, InputMode: tc.mode,
			}
			if got := BuildArgs(cfg, "/synthetic/record.mp4"); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("BuildArgs() = %#v; want %#v", got, tc.want)
			}
		})
	}
}

func TestClipboardShortcutUsesPhysicalVKeyAcrossKeyboardLayouts(t *testing.T) {
	script := clipboardShortcutScript(4242)
	for _, want := range []string{"unix id is 4242", "key code 9 using command down"} {
		if !strings.Contains(script, want) {
			t.Fatalf("script missing %q: %s", want, script)
		}
	}
	if strings.Contains(script, `keystroke "v"`) {
		t.Fatalf("layout-dependent keystroke returned: %s", script)
	}
}

func TestNewManagerDefersExecutableLookup(t *testing.T) {
	manager, err := NewManager(Config{
		Path:           "/definitely/missing/scrcpy",
		Target:         "synthetic-target",
		Package:        "com.samsung.android.messaging",
		Width:          1080,
		Height:         1920,
		StartupTimeout: time.Second,
	})
	if err != nil || manager == nil {
		t.Fatalf("NewManager() = %v, %v; want manager without executable lookup", manager, err)
	}
	if manager.config.InputMode != domain.TextInputIntentBody {
		t.Fatalf("default input mode=%q", manager.config.InputMode)
	}
}

type fakeProcess struct {
	logs   *io.PipeReader
	writer *io.PipeWriter
	wait   chan error

	mu        sync.Mutex
	waitCalls int
	signals   int
	kills     int
	signalErr error
	signal    func()
	kill      func()
	pid       int
}

func newFakeProcess() *fakeProcess {
	reader, writer := io.Pipe()
	return &fakeProcess{logs: reader, writer: writer, wait: make(chan error, 1), pid: 4242}
}

func (p *fakeProcess) LogReader() io.Reader { return p.logs }
func (p *fakeProcess) PID() int             { return p.pid }

func (p *fakeProcess) Wait() error {
	p.mu.Lock()
	p.waitCalls++
	p.mu.Unlock()
	return <-p.wait
}

func TestManagerSyncsClipboardThroughHealthyScrcpyClient(t *testing.T) {
	fake := newFakeProcess()
	fake.signal = func() { fake.complete(nil) }
	manager, _, _ := newTestManager(t, fake)
	manager.startProcess = func(context.Context, string, []string) (process, error) {
		go fake.writeLog("[server] INFO: New display: 1080x1920/344 (id=18)")
		return fake, nil
	}
	gotPID := 0
	manager.syncClipboard = func(_ context.Context, pid int) error {
		gotPID = pid
		return nil
	}
	if _, err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := manager.SyncClipboard(context.Background()); err != nil || gotPID != 4242 {
		t.Fatalf("pid=%d err=%v", gotPID, err)
	}
	if err := manager.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestManagerRejectsClipboardSyncWithoutHealthySession(t *testing.T) {
	manager, _, _ := newTestManager(t, newFakeProcess())
	if err := manager.SyncClipboard(context.Background()); !errors.Is(err, domain.ErrClipboardSync) {
		t.Fatalf("err=%v", err)
	}
}

func TestManagerRejectsClipboardSyncAfterProcessExit(t *testing.T) {
	fake := newFakeProcess()
	manager, _, _ := newTestManager(t, fake)
	manager.startProcess = func(context.Context, string, []string) (process, error) {
		go fake.writeLog("[server] INFO: New display: 1080x1920/344 (id=18)")
		return fake, nil
	}
	if _, err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	fake.complete(errors.New("synthetic exit"))
	select {
	case <-manager.session.done:
	case <-time.After(time.Second):
		t.Fatal("process exit was not observed")
	}
	err := manager.SyncClipboard(context.Background())
	if !errors.Is(err, domain.ErrClipboardSync) || !errors.Is(err, domain.ErrScrcpyExited) {
		t.Fatalf("err=%v", err)
	}
}

func TestManagerPreservesClipboardSyncCause(t *testing.T) {
	fake := newFakeProcess()
	fake.signal = func() { fake.complete(nil) }
	manager, _, _ := newTestManager(t, fake)
	manager.startProcess = func(context.Context, string, []string) (process, error) {
		go fake.writeLog("[server] INFO: New display: 1080x1920/344 (id=18)")
		return fake, nil
	}
	cause := errors.New("synthetic automation failure")
	manager.syncClipboard = func(context.Context, int) error { return cause }
	if _, err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	err := manager.SyncClipboard(context.Background())
	if !errors.Is(err, domain.ErrClipboardSync) || !errors.Is(err, cause) {
		t.Fatalf("err=%v", err)
	}
	if err := manager.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func (p *fakeProcess) Signal(os.Signal) error {
	p.mu.Lock()
	p.signals++
	err := p.signalErr
	signal := p.signal
	p.mu.Unlock()
	if signal != nil {
		signal()
	}
	return err
}

func (p *fakeProcess) Kill() error {
	p.mu.Lock()
	p.kills++
	kill := p.kill
	p.mu.Unlock()
	if kill != nil {
		kill()
	}
	return nil
}

func (p *fakeProcess) complete(err error) {
	_ = p.writer.Close()
	p.wait <- err
}

func (p *fakeProcess) writeLog(line string) {
	_, _ = p.writer.Write([]byte(line + "\n"))
}

func (p *fakeProcess) counts() (waits, signals, kills int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.waitCalls, p.signals, p.kills
}

func newTestManager(t *testing.T, fake *fakeProcess) (*Manager, string, *int) {
	t.Helper()
	manager, err := NewManager(Config{
		Path:           "/synthetic/scrcpy",
		Target:         "synthetic-target",
		Package:        "com.samsung.android.messaging",
		Width:          1080,
		Height:         1920,
		StartupTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}

	recordPath := filepath.Join(t.TempDir(), "record.mp4")
	starts := 0
	manager.lookupPath = func(string) (string, error) { return "/synthetic/scrcpy", nil }
	manager.createTemp = func() (*os.File, error) { return os.Create(recordPath) }
	manager.startProcess = func(context.Context, string, []string) (process, error) {
		starts++
		return fake, nil
	}
	return manager, recordPath, &starts
}

func TestManagerStartCachesDisplayAndStopIsIdempotent(t *testing.T) {
	fake := newFakeProcess()
	fake.signal = func() { fake.complete(nil) }
	manager, recordPath, starts := newTestManager(t, fake)
	manager.startProcess = func(context.Context, string, []string) (process, error) {
		(*starts)++
		go fake.writeLog("[server] INFO: New display: 1080x1920/344 (id=18)")
		return fake, nil
	}

	display, err := manager.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if display.AndroidDisplayID != 18 || display.Width != 1080 || display.Height != 1920 {
		t.Fatalf("display = %+v", display)
	}
	if !manager.Healthy(context.Background()) {
		t.Fatal("Healthy() = false after a successful start")
	}

	cached, err := manager.Start(context.Background())
	if err != nil || cached != display || *starts != 1 {
		t.Fatalf("cached=%+v err=%v starts=%d", cached, err, *starts)
	}

	if err := manager.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := manager.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(recordPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("record file error = %v; want not exist", err)
	}
	waits, signals, _ := fake.counts()
	if waits != 1 || signals != 1 || manager.Healthy(context.Background()) {
		t.Fatalf("waits=%d signals=%d healthy=%t", waits, signals, manager.Healthy(context.Background()))
	}
}

func TestManagerHealthyBecomesFalseAfterUnexpectedExit(t *testing.T) {
	fake := newFakeProcess()
	manager, _, starts := newTestManager(t, fake)
	manager.startProcess = func(context.Context, string, []string) (process, error) {
		(*starts)++
		go fake.writeLog("INFO: New display: 1080x1920/344 (id=18)")
		return fake, nil
	}
	if _, err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	fake.complete(errors.New("process exited"))
	waitFor(t, func() bool { return !manager.Healthy(context.Background()) })
	if *starts != 1 {
		t.Fatalf("starts=%d; want 1", *starts)
	}
}

func TestManagerRestartsAfterUnexpectedExit(t *testing.T) {
	first := newFakeProcess()
	second := newFakeProcess()
	second.signal = func() { second.complete(nil) }
	manager, _, starts := newTestManager(t, first)
	manager.startProcess = func(context.Context, string, []string) (process, error) {
		(*starts)++
		if *starts == 1 {
			go first.writeLog("INFO: New display: 1080x1920/344 (id=18)")
			return first, nil
		}
		go second.writeLog("INFO: New display: 1080x1920/344 (id=19)")
		return second, nil
	}
	if _, err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	first.complete(errors.New("unexpected exit"))
	waitFor(t, func() bool { return !manager.Healthy(context.Background()) })
	display, err := manager.Start(context.Background())
	if err != nil || display.AndroidDisplayID != 19 || *starts != 2 {
		t.Fatalf("display=%+v err=%v starts=%d", display, err, *starts)
	}
	if err := manager.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	firstWaits, _, _ := first.counts()
	secondWaits, _, _ := second.counts()
	if firstWaits != 1 || secondWaits != 1 {
		t.Fatalf("waits first=%d second=%d", firstWaits, secondWaits)
	}
}

func TestManagerStopDuringStartCancelsAndReapsProcess(t *testing.T) {
	fake := newFakeProcess()
	fake.kill = func() { fake.complete(nil) }
	manager, recordPath, _ := newTestManager(t, fake)
	started := make(chan struct{})
	manager.startProcess = func(context.Context, string, []string) (process, error) {
		close(started)
		return fake, nil
	}
	startResult := make(chan error, 1)
	go func() {
		_, err := manager.Start(context.Background())
		startResult <- err
	}()
	<-started
	stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := manager.Stop(stopCtx); err != nil {
		t.Fatal(err)
	}
	if err := <-startResult; !errors.Is(err, context.Canceled) || !errors.Is(err, domain.ErrScrcpyStartup) {
		t.Fatalf("start err=%v", err)
	}
	if manager.Healthy(context.Background()) {
		t.Fatal("manager remained healthy after Stop during Start")
	}
	if _, err := os.Stat(recordPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("record file error=%v", err)
	}
	waits, _, kills := fake.counts()
	if waits != 1 || kills != 1 {
		t.Fatalf("waits=%d kills=%d", waits, kills)
	}
}

func TestManagerStopBoundsGracefulWaitWithoutCallerDeadline(t *testing.T) {
	fake := newFakeProcess()
	fake.kill = func() { fake.complete(nil) }
	manager, _, starts := newTestManager(t, fake)
	manager.stopTimeout = 10 * time.Millisecond
	manager.startProcess = func(context.Context, string, []string) (process, error) {
		(*starts)++
		go fake.writeLog("INFO: New display: 1080x1920/344 (id=18)")
		return fake, nil
	}
	if _, err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}

	if err := manager.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, signals, kills := fake.counts()
	if signals != 1 || kills != 1 {
		t.Fatalf("signals=%d kills=%d; want graceful signal followed by kill", signals, kills)
	}
}

func TestManagerStopKillsAndReapsAfterSignalFailure(t *testing.T) {
	fake := newFakeProcess()
	fake.signalErr = errors.New("signal failed")
	fake.kill = func() { fake.complete(nil) }
	manager, _, starts := newTestManager(t, fake)
	manager.stopTimeout = 10 * time.Millisecond
	manager.startProcess = func(context.Context, string, []string) (process, error) {
		(*starts)++
		go fake.writeLog("INFO: New display: 1080x1920/344 (id=18)")
		return fake, nil
	}
	if _, err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}

	if err := manager.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	waits, signals, kills := fake.counts()
	if waits != 1 || signals != 1 || kills != 1 {
		t.Fatalf("waits=%d signals=%d kills=%d; want one of each", waits, signals, kills)
	}
}

func TestManagerStartTimeoutCleansUp(t *testing.T) {
	fake := newFakeProcess()
	fake.kill = func() { fake.complete(nil) }
	manager, recordPath, _ := newTestManager(t, fake)
	manager.config.StartupTimeout = 10 * time.Millisecond

	_, err := manager.Start(context.Background())
	if !errors.Is(err, domain.ErrScrcpyStartup) || !errors.Is(err, domain.ErrVirtualDisplayIDNotFound) {
		t.Fatalf("Start() error = %v", err)
	}
	assertFailedStartCleaned(t, manager, fake, recordPath, true)
}

func TestManagerStartProcessExitBeforeIDCleansUp(t *testing.T) {
	fake := newFakeProcess()
	manager, recordPath, _ := newTestManager(t, fake)
	manager.startProcess = func(context.Context, string, []string) (process, error) {
		go fake.complete(errors.New("process exited"))
		return fake, nil
	}

	_, err := manager.Start(context.Background())
	if !errors.Is(err, domain.ErrScrcpyStartup) || !errors.Is(err, domain.ErrScrcpyExited) {
		t.Fatalf("Start() error = %v", err)
	}
	assertFailedStartCleaned(t, manager, fake, recordPath, false)
}

func TestManagerStartContextCancellationCleansUp(t *testing.T) {
	fake := newFakeProcess()
	fake.kill = func() { fake.complete(nil) }
	manager, recordPath, _ := newTestManager(t, fake)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := manager.Start(ctx)
	if !errors.Is(err, domain.ErrScrcpyStartup) || !errors.Is(err, context.Canceled) {
		t.Fatalf("Start() error = %v", err)
	}
	assertFailedStartCleaned(t, manager, fake, recordPath, true)
}

func TestManagerStartMissingExecutableReturnsSafeRecognizableError(t *testing.T) {
	fake := newFakeProcess()
	manager, _, _ := newTestManager(t, fake)
	manager.lookupPath = func(string) (string, error) { return "", errors.New("missing") }

	_, err := manager.Start(context.Background())
	if !errors.Is(err, domain.ErrScrcpyNotFound) {
		t.Fatalf("Start() error = %v; want ErrScrcpyNotFound", err)
	}
	if strings.Contains(err.Error(), manager.config.Target) {
		t.Fatalf("Start() error exposes target: %q", err)
	}
}

func assertFailedStartCleaned(t *testing.T, manager *Manager, process *fakeProcess, recordPath string, wantKill bool) {
	t.Helper()
	if manager.Healthy(context.Background()) {
		t.Fatal("Healthy() = true after failed start")
	}
	if _, err := os.Stat(recordPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("record file error = %v; want not exist", err)
	}
	waits, _, kills := process.counts()
	if waits != 1 || (kills == 1) != wantKill {
		t.Fatalf("waits=%d kills=%d; want one wait and kill=%t", waits, kills, wantKill)
	}
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("condition was not met before timeout")
}
