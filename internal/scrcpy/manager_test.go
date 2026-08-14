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

func TestBuildArgs(t *testing.T) {
	cfg := Config{
		Target:  "synthetic-target",
		Package: "com.samsung.android.messaging",
		Width:   1080,
		Height:  1920,
	}

	got := BuildArgs(cfg, "/synthetic/record.mp4")
	want := []string{
		"-s", "synthetic-target",
		"--new-display=1080x1920",
		"--start-app=com.samsung.android.messaging",
		"--record=/synthetic/record.mp4",
		"--no-video-playback",
		"--no-audio",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BuildArgs() = %#v; want %#v", got, want)
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
}

func newFakeProcess() *fakeProcess {
	reader, writer := io.Pipe()
	return &fakeProcess{logs: reader, writer: writer, wait: make(chan error, 1)}
}

func (p *fakeProcess) LogReader() io.Reader { return p.logs }

func (p *fakeProcess) Wait() error {
	p.mu.Lock()
	p.waitCalls++
	p.mu.Unlock()
	return <-p.wait
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
