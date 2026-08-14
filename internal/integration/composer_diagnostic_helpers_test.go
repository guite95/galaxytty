//go:build integration

package integration

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/galaxytty/galaxytty/internal/domain"
)

func TestParseComposerDiagnosticStateScopesEvidenceToRuntimeDisplay(t *testing.T) {
	displayOutput := `
  Display 0:
    mBaseDisplayInfo=DisplayInfo{"main", displayId 0, state OFF, type INTERNAL}
  Display 27:
    mBaseDisplayInfo=DisplayInfo{"scrcpy", displayId 27, state ON, type VIRTUAL}
`
	activityOutput := `
Display #0 (activities from top to bottom):
  * Task{main A=com.samsung.android.messaging visible=false}
Display #27 (activities from top to bottom):
  * Task{diagnostic A=com.samsung.android.messaging visible=true}
    mResumedActivity: ActivityRecord{synthetic com.samsung.android.messaging/.ConversationComposer}
`
	inputOutput := `
	FocusedDisplayId: 0
	FocusedApplications:
	  displayId=0, name='ActivityRecord{synthetic com.sec.android.app.launcher/.Launcher}'
	FocusedWindows:
	  displayId=0, name='synthetic NotificationShade'
	FocusRequests:
	  FocusedDisplayId: 27
	  FocusedApplications:
    displayId=27, name='ActivityRecord{synthetic com.samsung.android.messaging/.ConversationComposer}'
    displayId=0, name='ActivityRecord{synthetic com.sec.android.app.launcher/.Launcher}'
  FocusedWindows:
    displayId=27, name='synthetic com.samsung.android.messaging/.ConversationComposer'
  FocusRequests:
`

	got := parseComposerDiagnosticState(27, displayOutput, activityOutput, inputOutput)
	want := composerDiagnosticState{
		displayState:       "ON",
		samsungTaskOnVD:    true,
		samsungResumedOnVD: true,
		focusedDisplay:     true,
		focusedApplication: true,
		focusedWindow:      true,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("state=%+v want=%+v", got, want)
	}
}

func TestParseComposerDiagnosticStateDoesNotBorrowMainDisplayFocus(t *testing.T) {
	got := parseComposerDiagnosticState(27,
		`  Display 27:
    mBaseDisplayInfo=DisplayInfo{"scrcpy", displayId 27, state OFF, type VIRTUAL}`,
		`Display #0 (activities from top to bottom):
  * Task{main A=com.samsung.android.messaging visible=true}
  mResumedActivity: ActivityRecord{synthetic com.samsung.android.messaging/.ConversationComposer}
Display #27 (activities from top to bottom):
  * Task{diagnostic A=com.sec.android.app.launcher visible=true}`,
		`FocusedDisplayId: 0
FocusedApplications:
  displayId=0, name='ActivityRecord{synthetic com.samsung.android.messaging/.ConversationComposer}'
FocusedWindows:
  displayId=0, name='synthetic com.samsung.android.messaging/.ConversationComposer'`,
	)
	want := composerDiagnosticState{displayState: "OFF"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("state=%+v want=%+v", got, want)
	}
}

func TestPrepareDiagnosticComposerHasNoSendCapability(t *testing.T) {
	controller := &recordingDiagnosticController{}
	display := domain.VirtualDisplay{AndroidDisplayID: 27, Width: 1080, Height: 1920}
	observations := 0
	afterSendTo, afterFocus, err := prepareDiagnosticComposer(
		context.Background(), controller, display, "synthetic-recipient", "synthetic-body", time.Nanosecond,
		func(context.Context) (composerDiagnosticState, error) {
			observations++
			return composerDiagnosticState{displayState: "stage-" + string(rune('0'+observations))}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"open", "focus"}
	if !reflect.DeepEqual(controller.actions, want) {
		t.Fatalf("actions=%v want=%v", controller.actions, want)
	}
	if afterSendTo.displayState != "stage-1" || afterFocus.displayState != "stage-2" {
		t.Fatalf("afterSendTo=%+v afterFocus=%+v", afterSendTo, afterFocus)
	}
}

func TestDiagnosticMarkerCreatedRequiresExactOutgoingRecipientAndBody(t *testing.T) {
	recipient := "01000000000"
	body := "GalaxyTTY DIAGNOSTIC synthetic"
	rows := []domain.Message{
		{Direction: domain.DirectionIncoming, Address: recipient, Body: body},
		{Direction: domain.DirectionOutgoing, Address: "01011111111", Body: body},
		{Direction: domain.DirectionOutgoing, Address: recipient, Body: "different"},
	}
	if diagnosticMarkerCreated(rows, recipient, body) {
		t.Fatal("non-exact provider evidence was accepted as a diagnostic send")
	}
	rows = append(rows, domain.Message{Direction: domain.DirectionOutgoing, Address: "+82 10-0000-0000", Body: body})
	if !diagnosticMarkerCreated(rows, recipient, body) {
		t.Fatal("exact normalized outgoing provider evidence was not detected")
	}
}

type recordingDiagnosticController struct {
	actions []string
}

func (c *recordingDiagnosticController) OpenConversationWithBody(context.Context, domain.VirtualDisplay, string, string) error {
	c.actions = append(c.actions, "open")
	return nil
}

func (c *recordingDiagnosticController) FocusComposer(context.Context, domain.VirtualDisplay) error {
	c.actions = append(c.actions, "focus")
	return nil
}
