//go:build integration

package integration

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/galaxytty/galaxytty/internal/adb"
	"github.com/galaxytty/galaxytty/internal/bootstrap"
	"github.com/galaxytty/galaxytty/internal/config"
	"github.com/galaxytty/galaxytty/internal/domain"
	"github.com/galaxytty/galaxytty/internal/provider"
	"github.com/galaxytty/galaxytty/internal/samsung"
	"github.com/galaxytty/galaxytty/internal/scrcpy"
)

const realSamsungMessagesPackage = "com.samsung.android.messaging"

func TestRealVirtualDisplaySmoke(t *testing.T) {
	if os.Getenv("GALAXYTTY_REAL_DISPLAY_TEST") != "1" {
		t.Skip("explicit real virtual display test opt-in required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cfg := config.Default()
	target := discoverTarget(t, ctx, cfg)
	beforeState := readMainDisplayState(ctx, target)
	beforeRecords := recordingSnapshot(t)

	manager, err := scrcpy.NewManager(scrcpy.Config{
		Target: target.Info().Serial, Package: realSamsungMessagesPackage,
		Width: cfg.Samsung.DisplayWidth, Height: cfg.Samsung.DisplayHeight,
		StartupTimeout: 15 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	display, err := manager.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	stopped := false
	defer func() {
		if !stopped {
			stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer stopCancel()
			_ = manager.Stop(stopCtx)
		}
	}()
	if display.AndroidDisplayID <= 0 || !manager.Healthy(ctx) {
		t.Fatal("virtual display did not become healthy with a runtime logical ID")
	}
	if recipient := strings.TrimSpace(os.Getenv("GALAXYTTY_TEST_RECIPIENT")); recipient != "" {
		controller, err := samsung.NewController(target, samsung.DefaultLayout())
		if err != nil {
			t.Fatal(err)
		}
		if err := controller.OpenConversation(ctx, display, recipient); err != nil {
			t.Fatal("Samsung Messages conversation open smoke failed")
		}
	}
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	err = manager.Stop(stopCtx)
	stopCancel()
	if err != nil || manager.Healthy(context.Background()) {
		t.Fatalf("virtual display stop failed: healthy=%t err=%v", manager.Healthy(context.Background()), err)
	}
	stopped = true
	assertNoNewRecordings(t, beforeRecords)
	assertMainDisplayStatePreserved(t, beforeState, readMainDisplayState(ctx, target))
}

func TestRealSamsungSend(t *testing.T) {
	if os.Getenv("GALAXYTTY_ENABLE_SEND_TEST") != "1" || strings.TrimSpace(os.Getenv("GALAXYTTY_TEST_RECIPIENT")) == "" {
		t.Skip("explicit real send test opt-in and recipient required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cfg := config.Default()
	recipient := strings.TrimSpace(os.Getenv("GALAXYTTY_TEST_RECIPIENT"))
	body := "GalaxyTTY integration test 한글 😀 " + time.Now().UTC().Format(time.RFC3339)
	target := discoverTarget(t, ctx, cfg)
	store := provider.NewStore(target)
	baseline, err := store.LatestMessageID(ctx)
	if err != nil {
		t.Fatal("could not establish provider baseline")
	}
	beforeState := readMainDisplayState(ctx, target)
	beforeRecords := recordingSnapshot(t)

	runtime, err := bootstrap.Real(ctx, cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	shutdown := false
	defer func() {
		if !shutdown {
			stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer stopCancel()
			_ = runtime.Service.Shutdown(stopCtx)
		}
	}()
	result, err := runtime.Service.SendToAddress(ctx, recipient, body)
	if err != nil {
		t.Fatalf("real Samsung Messages send was not verified: %v", err)
	}
	if result.MessageID <= baseline || result.ThreadID <= 0 {
		t.Fatal("verified send returned invalid provider IDs")
	}

	rows, err := store.MessagesAfter(ctx, baseline)
	if err != nil {
		t.Fatal("provider post-send verification query failed")
	}
	found := false
	for _, message := range rows {
		if message.ID == result.MessageID && message.ThreadID == result.ThreadID && message.Direction == domain.DirectionOutgoing && domain.NormalizePhone(message.Address) == domain.NormalizePhone(recipient) && message.Body == body {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("verified outgoing provider row was not found after send")
	}
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	err = runtime.Service.Shutdown(stopCtx)
	stopCancel()
	if err != nil {
		t.Fatal("real send runtime shutdown failed")
	}
	shutdown = true
	assertNoNewRecordings(t, beforeRecords)
	assertMainDisplayStatePreserved(t, beforeState, readMainDisplayState(ctx, target))
}

func discoverTarget(t *testing.T, ctx context.Context, cfg config.Config) *adb.Target {
	t.Helper()
	client, err := adb.NewClient("", 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	target, err := adb.Discover(ctx, client, adb.SelectionOptions{PreferUSB: cfg.Connection.PreferUSB, Target: cfg.Connection.Device})
	if err != nil {
		t.Fatal(err)
	}
	return target
}

type mainDisplayState struct {
	locked, lockedKnown bool
	off, offKnown       bool
}

func TestParseMainDisplayStateRecognizesReferenceDeviceOutput(t *testing.T) {
	state := parseMainDisplayState(
		"trustState=UNTRUSTED, deviceLocked=1",
		"mDreamingLockscreen=true isKeyguardShowing=true",
		"mWakefulness=Dozing mScreenState=OFF",
	)
	if !state.lockedKnown || !state.locked || !state.offKnown || !state.off {
		t.Fatalf("state=%+v", state)
	}
}

func readMainDisplayState(ctx context.Context, target *adb.Target) mainDisplayState {
	trust, _ := target.Shell(ctx, "dumpsys", "trust")
	window, err := target.Shell(ctx, "dumpsys", "window", "policy")
	if err != nil {
		window = nil
	}
	power, err := target.Shell(ctx, "dumpsys", "power")
	if err != nil {
		power = nil
	}
	return parseMainDisplayState(string(trust), string(window), string(power))
}

func parseMainDisplayState(trust, window, power string) mainDisplayState {
	var state mainDisplayState
	lockValue := strings.ToLower(trust + " " + window)
	switch {
	case strings.Contains(lockValue, "devicelocked=1"), strings.Contains(lockValue, "deviceislocked=true"), strings.Contains(lockValue, "mshowinglockscreen=true"), strings.Contains(lockValue, "isdreaminglockscreen=true"), strings.Contains(lockValue, "isstatusbarkeyguard=true"), strings.Contains(lockValue, "iskeyguardshowing=true"):
		state.locked, state.lockedKnown = true, true
	case strings.Contains(lockValue, "devicelocked=0"), strings.Contains(lockValue, "deviceislocked=false"), strings.Contains(lockValue, "mshowinglockscreen=false"), strings.Contains(lockValue, "isdreaminglockscreen=false"), strings.Contains(lockValue, "isstatusbarkeyguard=false"), strings.Contains(lockValue, "iskeyguardshowing=false"):
		state.lockedKnown = true
	}
	powerValue := strings.ToLower(power)
	switch {
	case strings.Contains(powerValue, "display power: state=off"), strings.Contains(powerValue, "mwakefulness=asleep"), strings.Contains(powerValue, "mwakefulness=dozing"), strings.Contains(powerValue, "mscreenstate=off"):
		state.off, state.offKnown = true, true
	case strings.Contains(powerValue, "display power: state=on"), strings.Contains(powerValue, "mwakefulness=awake"), strings.Contains(powerValue, "mscreenstate=on"):
		state.offKnown = true
	}
	return state
}

func assertMainDisplayStatePreserved(t *testing.T, before, after mainDisplayState) {
	t.Helper()
	if before.lockedKnown && before.locked && after.lockedKnown && !after.locked {
		t.Fatal("main display became unlocked during virtual-display send flow")
	}
	if before.offKnown && before.off && after.offKnown && !after.off {
		t.Fatal("main display became active during virtual-display send flow")
	}
}

func recordingSnapshot(t *testing.T) map[string]struct{} {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(os.TempDir(), "galaxytty-scrcpy-*.mp4"))
	if err != nil {
		t.Fatal(err)
	}
	snapshot := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		snapshot[path] = struct{}{}
	}
	return snapshot
}

func assertNoNewRecordings(t *testing.T, before map[string]struct{}) {
	t.Helper()
	after := recordingSnapshot(t)
	for path := range after {
		if _, existed := before[path]; !existed {
			t.Fatal("scrcpy temporary recording was not cleaned up")
		}
	}
}
