//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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

func TestRealSamsungRCSSend(t *testing.T) {
	if os.Getenv("GALAXYTTY_ENABLE_RCS_SEND_TEST") != "1" || strings.TrimSpace(os.Getenv("GALAXYTTY_RCS_TEST_RECIPIENT")) == "" {
		t.Skip("explicit real RCS send test opt-in and recipient required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cfg := config.Default()
	recipient := strings.TrimSpace(os.Getenv("GALAXYTTY_RCS_TEST_RECIPIENT"))
	body := "GalaxyTTY RCS integration test " + time.Now().UTC().Format(time.RFC3339)
	target := discoverTarget(t, ctx, cfg)
	store := provider.NewStore(target)
	baseline, err := store.LatestMessageID(ctx)
	if err != nil {
		t.Fatal("could not establish RCS observation baseline")
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
	observation, err := collectRCSSendObservation(
		func() (domain.SendResult, error) {
			return runtime.Service.SendToAddress(ctx, recipient, body)
		},
		func() (rcsProviderObservation, error) {
			return observeRCSProviderEvidence(ctx, target.Shell, baseline, recipient, body)
		},
	)
	if err != nil {
		t.Fatalf("RCS send or provider observation failed; no retry was attempted: %v", err)
	}
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	err = runtime.Service.Shutdown(stopCtx)
	stopCancel()
	if err != nil {
		t.Fatal("RCS observation runtime shutdown failed")
	}
	shutdown = true
	assertNoNewRecordings(t, beforeRecords)
	assertMainDisplayStatePreserved(t, beforeState, readMainDisplayState(ctx, target))

	if observation.result.Transport == domain.MessageRCS {
		return
	}
	supported, nonEmpty := observation.evidence.extensions.counts()
	if observation.evidence.exactOutgoing {
		t.Skipf("exact outgoing provider evidence exists, but %d supported extension fields (%d non-empty) do not reliably classify RCS transport", supported, nonEmpty)
	}
	if errors.Is(observation.sendErr, domain.ErrSendVerificationTimeout) {
		t.Skipf("RCS verification timed out; post-send observation found exact_correlation=%t outgoing=%t and %d supported extension fields (%d non-empty)", observation.evidence.exactCorrelation, observation.evidence.exactOutgoing, supported, nonEmpty)
	}
	t.Skipf("accessible evidence did not reliably classify RCS transport; exact_correlation=%t outgoing=%t", observation.evidence.exactCorrelation, observation.evidence.exactOutgoing)
}

func TestRealSamsungMMSTextSend(t *testing.T) {
	if os.Getenv("GALAXYTTY_ENABLE_MMS_SEND_TEST") != "1" || strings.TrimSpace(os.Getenv("GALAXYTTY_MMS_TEST_RECIPIENT")) == "" {
		t.Skip("explicit real MMS text send test opt-in and recipient required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cfg := config.Default()
	recipient := strings.TrimSpace(os.Getenv("GALAXYTTY_MMS_TEST_RECIPIENT"))
	prefix := "GalaxyTTY MMS integration test " + time.Now().UTC().Format(time.RFC3339) + " "
	body := prefix + strings.Repeat("MMS text verification 한글 😀 ", 48)
	target := discoverTarget(t, ctx, cfg)
	store := provider.NewStore(target)
	baseline, err := store.LatestMMSMessageID(ctx)
	if err != nil {
		t.Fatal("could not establish MMS provider baseline")
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
		t.Fatalf("MMS text send was not machine verified; no retry was attempted: %v", err)
	}
	if result.Transport != domain.MessageMMS || result.MessageID <= baseline {
		t.Fatal("Samsung Messages did not expose exact outgoing MMS text evidence")
	}
	messages, err := store.MMSMessagesAfter(ctx, baseline)
	if err != nil {
		t.Fatal("MMS provider post-send verification query failed")
	}
	found := false
	for _, message := range messages {
		if message.ID == result.MessageID && message.ThreadID == result.ThreadID && message.Direction == domain.DirectionOutgoing && message.Type == domain.MessageMMS && domain.NormalizePhone(message.Address) == domain.NormalizePhone(recipient) && message.Body == body {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("verified outgoing MMS provider evidence was not found after send")
	}
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	err = runtime.Service.Shutdown(stopCtx)
	stopCancel()
	if err != nil {
		t.Fatal("MMS send runtime shutdown failed")
	}
	shutdown = true
	assertNoNewRecordings(t, beforeRecords)
	assertMainDisplayStatePreserved(t, beforeState, readMainDisplayState(ctx, target))
}

type smsExtensionSnapshot struct {
	supported map[string]bool
	nonEmpty  map[string]bool
}

type shellQuery func(context.Context, ...string) ([]byte, error)

type rcsProviderObservation struct {
	exactCorrelation bool
	exactOutgoing    bool
	messageID        int64
	smsType          int
	extensions       smsExtensionSnapshot
}

type rcsSendObservation struct {
	result   domain.SendResult
	sendErr  error
	evidence rcsProviderObservation
}

func collectRCSSendObservation(send func() (domain.SendResult, error), observe func() (rcsProviderObservation, error)) (rcsSendObservation, error) {
	result, sendErr := send()
	if sendErr != nil && !errors.Is(sendErr, domain.ErrSendVerificationTimeout) {
		return rcsSendObservation{}, sendErr
	}
	if sendErr == nil && result.Transport == domain.MessageRCS {
		return rcsSendObservation{result: result}, nil
	}
	evidence, err := observe()
	if err != nil {
		return rcsSendObservation{}, err
	}
	return rcsSendObservation{result: result, sendErr: sendErr, evidence: evidence}, nil
}

func observeRCSProviderEvidence(ctx context.Context, shell shellQuery, baseline int64, recipient, body string) (rcsProviderObservation, error) {
	columns := []string{"_id", "address", "type", "body"}
	output, err := shell(ctx,
		"content", "query", "--uri", "content://sms",
		"--projection", strings.Join(columns, ":"),
		"--where", fmt.Sprintf(`"_id > %d"`, baseline),
		"--sort", `"_id ASC LIMIT 500"`,
	)
	if err != nil {
		return rcsProviderObservation{}, fmt.Errorf("query post-send SMS evidence: %w", err)
	}
	if strings.TrimSpace(string(output)) == "No result found." {
		return rcsProviderObservation{extensions: newSMSExtensionSnapshot()}, nil
	}
	rows, err := provider.ParseProjectedRows(string(output), columns, "body")
	if err != nil {
		return rcsProviderObservation{}, fmt.Errorf("parse post-send SMS evidence: %w", err)
	}
	observation, err := findRCSProviderCorrelation(rows, baseline, recipient, body)
	if err != nil {
		return rcsProviderObservation{}, err
	}
	if observation.messageID > 0 {
		observation.extensions, err = observeSMSExtensions(ctx, shell, observation.messageID)
		if err != nil {
			return rcsProviderObservation{}, err
		}
	} else {
		observation.extensions = newSMSExtensionSnapshot()
	}
	return observation, nil
}

func findRCSProviderCorrelation(rows []map[string]string, baseline int64, recipient, body string) (rcsProviderObservation, error) {
	var observation rcsProviderObservation
	for _, row := range rows {
		messageID, err := strconv.ParseInt(row["_id"], 10, 64)
		if err != nil {
			return rcsProviderObservation{}, fmt.Errorf("parse post-send SMS message ID")
		}
		smsType, err := strconv.Atoi(row["type"])
		if err != nil {
			return rcsProviderObservation{}, fmt.Errorf("parse post-send SMS type")
		}
		if messageID <= baseline || domain.NormalizePhone(row["address"]) != domain.NormalizePhone(recipient) || row["body"] != body {
			continue
		}
		observation = rcsProviderObservation{
			exactCorrelation: true,
			exactOutgoing:    smsType == 2,
			messageID:        messageID,
			smsType:          smsType,
		}
	}
	return observation, nil
}

func newSMSExtensionSnapshot() smsExtensionSnapshot {
	return smsExtensionSnapshot{supported: map[string]bool{}, nonEmpty: map[string]bool{}}
}

func (s smsExtensionSnapshot) counts() (supported, nonEmpty int) {
	for _, present := range s.supported {
		if present {
			supported++
		}
	}
	for _, present := range s.nonEmpty {
		if present {
			nonEmpty++
		}
	}
	return supported, nonEmpty
}

func observeSMSExtensions(ctx context.Context, shell shellQuery, messageID int64) (smsExtensionSnapshot, error) {
	snapshot := newSMSExtensionSnapshot()
	if messageID <= 0 {
		return snapshot, nil
	}
	for _, field := range []string{"teleservice_id", "app_id", "chat_type", "correlation_tag"} {
		output, err := shell(ctx,
			"content", "query", "--uri", "content://sms",
			"--projection", "_id:"+field,
			"--where", `"_id = `+strconv.FormatInt(messageID, 10)+`"`,
		)
		if err != nil {
			return snapshot, fmt.Errorf("query SMS extension field %s: %w", field, err)
		}
		rows, err := provider.ParseContentRows(string(output), []string{"_id", field})
		if err != nil || len(rows) != 1 {
			continue
		}
		snapshot.supported[field] = true
		value := strings.TrimSpace(rows[0][field])
		snapshot.nonEmpty[field] = value != "" && !strings.EqualFold(value, "null")
	}
	return snapshot, nil
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
