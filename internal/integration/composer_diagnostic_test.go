//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/galaxytty/galaxytty/internal/adb"
	"github.com/galaxytty/galaxytty/internal/config"
	"github.com/galaxytty/galaxytty/internal/domain"
	"github.com/galaxytty/galaxytty/internal/provider"
	"github.com/galaxytty/galaxytty/internal/samsung"
	"github.com/galaxytty/galaxytty/internal/scrcpy"
)

const composerDiagnosticDelay = 500 * time.Millisecond

func TestRealSamsungComposerDiagnostic(t *testing.T) {
	if os.Getenv("GALAXYTTY_ENABLE_COMPOSER_DIAGNOSTIC") != "1" || strings.TrimSpace(os.Getenv("GALAXYTTY_DIAGNOSTIC_RECIPIENT")) == "" {
		t.Skip("explicit non-sending composer diagnostic opt-in and recipient required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	cfg := config.Default()
	recipient := strings.TrimSpace(os.Getenv("GALAXYTTY_DIAGNOSTIC_RECIPIENT"))
	target := discoverTarget(t, ctx, cfg)
	store := provider.NewStore(target)
	smsBaseline, err := store.LatestMessageID(ctx)
	if err != nil {
		t.Fatalf("could not establish diagnostic SMS baseline: %s", safeProviderDiagnostic(err))
	}
	mmsBaseline, err := store.LatestMMSMessageID(ctx)
	if err != nil {
		t.Fatalf("could not establish diagnostic MMS baseline: %s", safeProviderDiagnostic(err))
	}
	mainBefore := readMainDisplayState(ctx, target)
	recordingsBefore := recordingSnapshot(t)

	timestamp := time.Now().UTC().Format("20060102T150405Z")
	variants := []struct {
		name       string
		keepActive bool
		body       string
	}{
		{name: "CURRENT", body: "GalaxyTTY DIAGNOSTIC CURRENT 한글 😀 " + timestamp},
		{name: "KEEP_ACTIVE", keepActive: true, body: "GalaxyTTY DIAGNOSTIC KEEP ACTIVE 한글 😀 " + timestamp},
	}
	results := make([]composerDiagnosticResult, 0, len(variants))
	for _, variant := range variants {
		result, err := runComposerDiagnosticVariant(ctx, target, cfg, recipient, variant.body, variant.keepActive)
		if err != nil {
			t.Fatalf("%s composer diagnostic failed without a send action: %s", variant.name, safeComposerDiagnosticError(err))
		}
		results = append(results, result)
		logComposerDiagnosticResult(t, variant.name, result)
	}

	smsRows, err := store.MessagesAfter(ctx, smsBaseline)
	if err != nil {
		t.Fatalf("could not verify diagnostic SMS non-send result: %s", safeProviderDiagnostic(err))
	}
	mmsRows, err := store.MMSMessagesAfter(ctx, mmsBaseline)
	if err != nil {
		t.Fatalf("could not verify diagnostic MMS non-send result: %s", safeProviderDiagnostic(err))
	}
	for _, variant := range variants {
		if diagnosticMarkerCreated(smsRows, recipient, variant.body) || diagnosticMarkerCreated(mmsRows, recipient, variant.body) {
			t.Fatal("diagnostic marker unexpectedly produced outgoing provider evidence")
		}
		observation, err := observeRCSProviderEvidence(ctx, target.Shell, smsBaseline, recipient, variant.body)
		if err != nil {
			t.Fatalf("could not inspect diagnostic provider correlation: %s", safeProviderDiagnostic(err))
		}
		if observation.exactCorrelation {
			t.Fatal("diagnostic marker unexpectedly persisted in the SMS provider")
		}
	}

	assertNoNewRecordings(t, recordingsBefore)
	assertMainDisplayStatePreserved(t, mainBefore, readMainDisplayState(ctx, target))
	t.Log("ACTUAL_MESSAGE_CREATED=false")
	t.Logf("KEEP_ACTIVE_DIFFERENCE=%s", classifyKeepActiveDifference(results[0], results[1]))
}

type composerDiagnosticResult struct {
	started           bool
	afterStartup      composerDiagnosticState
	afterReadyDelay   composerDiagnosticState
	afterSendTo       composerDiagnosticState
	afterFocus        composerDiagnosticState
	sendToAccepted    bool
	focusAccepted     bool
	composerCleared   bool
	stopped           bool
	displayRemoved    bool
	mainDisplayStable bool
}

func runComposerDiagnosticVariant(
	ctx context.Context,
	target *adb.Target,
	cfg config.Config,
	recipient, body string,
	keepActive bool,
) (result composerDiagnosticResult, err error) {
	mainBefore := readMainDisplayState(ctx, target)
	manager, err := scrcpy.NewManager(scrcpy.Config{
		Target: target.Info().Serial, Package: realSamsungMessagesPackage,
		Width: cfg.Samsung.DisplayWidth, Height: cfg.Samsung.DisplayHeight,
		StartupTimeout: 15 * time.Second,
		InputMode:      domain.TextInputIntentBody,
		KeepActive:     keepActive,
	})
	if err != nil {
		return result, err
	}
	controller, err := samsung.NewController(target, samsung.DefaultLayout())
	if err != nil {
		return result, err
	}
	if err := controller.EnsureDefaultSMSHandler(ctx); err != nil {
		return result, err
	}
	display, err := manager.Start(ctx)
	if err != nil {
		return result, err
	}
	result.started = true
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if stopErr := manager.Stop(stopCtx); stopErr != nil && err == nil {
			err = fmt.Errorf("stop diagnostic virtual display: %w", stopErr)
		}
		result.stopped = !manager.Healthy(context.Background())
		result.displayRemoved = readDiagnosticDisplayState(context.Background(), target, display.AndroidDisplayID) == "UNKNOWN"
		mainAfter := readMainDisplayState(context.Background(), target)
		result.mainDisplayStable = mainDisplayStatePreserved(mainBefore, mainAfter)
	}()

	result.afterStartup, err = readComposerDiagnosticState(ctx, target, display.AndroidDisplayID)
	if err != nil {
		return result, err
	}
	if err := waitDiagnostic(ctx, composerDiagnosticDelay); err != nil {
		return result, err
	}
	result.afterReadyDelay, err = readComposerDiagnosticState(ctx, target, display.AndroidDisplayID)
	if err != nil {
		return result, err
	}
	result.afterSendTo, result.afterFocus, err = prepareDiagnosticComposer(
		ctx, controller, display, recipient, body, composerDiagnosticDelay,
		func(observeCtx context.Context) (composerDiagnosticState, error) {
			return readComposerDiagnosticState(observeCtx, target, display.AndroidDisplayID)
		},
	)
	if err != nil {
		return result, err
	}
	result.sendToAccepted = true
	result.focusAccepted = true
	result.composerCleared = controller.ClearComposer(ctx, display) == nil
	return result, nil
}

func readComposerDiagnosticState(ctx context.Context, target *adb.Target, displayID int64) (composerDiagnosticState, error) {
	displayOutput, err := target.Shell(ctx, "dumpsys", "display")
	if err != nil {
		return composerDiagnosticState{}, fmt.Errorf("read display diagnostic state: %w", err)
	}
	activityOutput, err := target.Shell(ctx, "dumpsys", "activity", "activities")
	if err != nil {
		return composerDiagnosticState{}, fmt.Errorf("read activity diagnostic state: %w", err)
	}
	inputOutput, err := target.Shell(ctx, "dumpsys", "input")
	if err != nil {
		return composerDiagnosticState{}, fmt.Errorf("read input diagnostic state: %w", err)
	}
	return parseComposerDiagnosticState(displayID, string(displayOutput), string(activityOutput), string(inputOutput)), nil
}

func readDiagnosticDisplayState(ctx context.Context, target *adb.Target, displayID int64) string {
	readCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	output, err := target.Shell(readCtx, "dumpsys", "display")
	if err != nil {
		return "UNKNOWN"
	}
	return parseComposerDiagnosticState(displayID, string(output), "", "").displayState
}

func mainDisplayStatePreserved(before, after mainDisplayState) bool {
	if before.lockedKnown && before.locked && after.lockedKnown && !after.locked {
		return false
	}
	if before.offKnown && before.off && after.offKnown && !after.off {
		return false
	}
	return true
}

func safeComposerDiagnosticError(err error) string {
	safe := safeProviderDiagnostic(err)
	if safe != "provider query failed" {
		return safe
	}
	for _, candidate := range []error{
		domain.ErrScrcpyNotFound,
		domain.ErrScrcpyStartup,
		domain.ErrScrcpyExited,
		domain.ErrVirtualDisplayIDNotFound,
		domain.ErrSamsungMessagesNotDefault,
		domain.ErrConversationOpen,
		domain.ErrComposerTap,
		domain.ErrComposerClear,
	} {
		if errors.Is(err, candidate) {
			return candidate.Error()
		}
	}
	return "composer diagnostic failed"
}

func logComposerDiagnosticResult(t *testing.T, mode string, result composerDiagnosticResult) {
	t.Helper()
	t.Logf("MODE=%s VD_STARTED=%t START_STATE=%s READY_STATE=%s SENDTO_STATE=%s FOCUS_STATE=%s SAMSUNG_TASK_ON_VD=%t SAMSUNG_RESUMED_ON_VD=%t CURRENT_FOCUSED_DISPLAY=%t CURRENT_FOCUSED_APPLICATION=%t CURRENT_FOCUSED_WINDOW=%t SENDTO_ACCEPTED=%t FOCUS_ACCEPTED=%t COMPOSER_READY=%t COMPOSER_CLEARED=%t VD_STOPPED=%t VD_REMOVED=%t MAIN_DISPLAY_PRESERVED=%t",
		mode,
		result.started,
		result.afterStartup.displayState,
		result.afterReadyDelay.displayState,
		result.afterSendTo.displayState,
		result.afterFocus.displayState,
		result.afterFocus.samsungTaskOnVD,
		result.afterFocus.samsungResumedOnVD,
		result.afterFocus.currentFocusedDisplay,
		result.afterFocus.currentFocusedApplication,
		result.afterFocus.currentFocusedWindow,
		result.sendToAccepted,
		result.focusAccepted,
		result.afterFocus.composerReady(),
		result.composerCleared,
		result.stopped,
		result.displayRemoved,
		result.mainDisplayStable,
	)
}

func classifyKeepActiveDifference(current, keepActive composerDiagnosticResult) string {
	currentReady := current.afterFocus.composerReady()
	keepActiveReady := keepActive.afterFocus.composerReady()
	switch {
	case !currentReady && keepActiveReady:
		return "KEEP_ACTIVE_IMPROVED_READINESS"
	case currentReady && keepActiveReady:
		return "BOTH_READY"
	case currentReady && !keepActiveReady:
		return "KEEP_ACTIVE_REDUCED_READINESS"
	default:
		return "BOTH_NOT_READY"
	}
}

type composerDiagnosticState struct {
	displayState              string
	samsungTaskOnVD           bool
	samsungResumedOnVD        bool
	currentFocusedDisplay     bool
	currentFocusedApplication bool
	currentFocusedWindow      bool
}

func (s composerDiagnosticState) composerReady() bool {
	return s.displayState == "ON" &&
		s.samsungTaskOnVD &&
		s.samsungResumedOnVD &&
		s.currentFocusedDisplay &&
		s.currentFocusedApplication &&
		s.currentFocusedWindow
}

func parseComposerDiagnosticState(displayID int64, displayOutput, activityOutput, inputOutput string) composerDiagnosticState {
	id := strconv.FormatInt(displayID, 10)
	displaySection := diagnosticSection(displayOutput, `(?m)^\s{2}Display `+regexp.QuoteMeta(id)+`:\s*$`, `(?m)^\s{2}Display \d+:\s*$`)
	activitySection := diagnosticSection(activityOutput, `(?m)^Display #`+regexp.QuoteMeta(id)+`\s`, `(?m)^Display #\d+\s`)
	state := composerDiagnosticState{displayState: parseDiagnosticDisplayState(displaySection)}
	state.samsungTaskOnVD = strings.Contains(activitySection, realSamsungMessagesPackage)
	state.samsungResumedOnVD = diagnosticLineContains(activitySection, realSamsungMessagesPackage, "mResumedActivity", "topResumedActivity")
	currentInput := currentInputSnapshot(inputOutput)
	state.currentFocusedDisplay = regexp.MustCompile(`(?m)^\s*FocusedDisplayId:\s*` + regexp.QuoteMeta(id) + `\s*$`).MatchString(currentInput)
	state.currentFocusedApplication = diagnosticDisplayLineContains(currentInputBlock(currentInput, "FocusedApplications:", "FocusedWindows:"), id, realSamsungMessagesPackage)
	state.currentFocusedWindow = diagnosticDisplayLineContains(currentInputBlock(currentInput, "FocusedWindows:", ""), id, realSamsungMessagesPackage)
	return state
}

func diagnosticSection(output, startPattern, nextPattern string) string {
	start := regexp.MustCompile(startPattern).FindStringIndex(output)
	if start == nil {
		return ""
	}
	rest := output[start[1]:]
	next := regexp.MustCompile(nextPattern).FindStringIndex(rest)
	if next == nil {
		return output[start[0]:]
	}
	return output[start[0] : start[1]+next[0]]
}

func currentInputSnapshot(output string) string {
	if index := regexp.MustCompile(`(?m)^\s*FocusRequests:\s*$`).FindStringIndex(output); index != nil {
		return output[:index[0]]
	}
	return output
}

func currentInputBlock(output, start, end string) string {
	startIndex := strings.Index(output, start)
	if startIndex < 0 {
		return ""
	}
	rest := output[startIndex+len(start):]
	if end == "" {
		return rest
	}
	endIndex := strings.Index(rest, end)
	if endIndex < 0 {
		return ""
	}
	return rest[:endIndex]
}

func parseDiagnosticDisplayState(section string) string {
	for _, line := range strings.Split(section, "\n") {
		if !strings.Contains(line, "mBaseDisplayInfo=") {
			continue
		}
		match := regexp.MustCompile(`\bstate\s+([A-Za-z_]+)`).FindStringSubmatch(line)
		if len(match) == 2 {
			return strings.ToUpper(match[1])
		}
	}
	return "UNKNOWN"
}

func diagnosticLineContains(output, packageName string, prefixes ...string) bool {
	for _, line := range strings.Split(output, "\n") {
		if !strings.Contains(line, packageName) {
			continue
		}
		for _, prefix := range prefixes {
			if strings.Contains(line, prefix) {
				return true
			}
		}
	}
	return false
}

func diagnosticDisplayLineContains(output, displayID, value string) bool {
	needle := "displayId=" + displayID
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, needle) && strings.Contains(line, value) {
			return true
		}
	}
	return false
}

type diagnosticComposerController interface {
	OpenConversationWithBody(context.Context, domain.VirtualDisplay, string, string) error
	FocusComposer(context.Context, domain.VirtualDisplay) error
}

func prepareDiagnosticComposer(
	ctx context.Context,
	controller diagnosticComposerController,
	display domain.VirtualDisplay,
	recipient, body string,
	delay time.Duration,
	observe func(context.Context) (composerDiagnosticState, error),
) (composerDiagnosticState, composerDiagnosticState, error) {
	if err := controller.OpenConversationWithBody(ctx, display, recipient, body); err != nil {
		return composerDiagnosticState{}, composerDiagnosticState{}, fmt.Errorf("open diagnostic composer: %w", err)
	}
	if err := waitDiagnostic(ctx, delay); err != nil {
		return composerDiagnosticState{}, composerDiagnosticState{}, err
	}
	afterSendTo, err := observe(ctx)
	if err != nil {
		return composerDiagnosticState{}, composerDiagnosticState{}, err
	}
	if err := controller.FocusComposer(ctx, display); err != nil {
		return composerDiagnosticState{}, composerDiagnosticState{}, fmt.Errorf("focus diagnostic composer: %w", err)
	}
	if err := waitDiagnostic(ctx, delay); err != nil {
		return composerDiagnosticState{}, composerDiagnosticState{}, err
	}
	afterFocus, err := observe(ctx)
	if err != nil {
		return composerDiagnosticState{}, composerDiagnosticState{}, err
	}
	return afterSendTo, afterFocus, nil
}

func waitDiagnostic(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func diagnosticMarkerCreated(messages []domain.Message, recipient, body string) bool {
	recipient = domain.NormalizePhone(recipient)
	for _, message := range messages {
		if message.Direction == domain.DirectionOutgoing &&
			domain.NormalizePhone(message.Address) == recipient &&
			message.Body == body {
			return true
		}
	}
	return false
}
