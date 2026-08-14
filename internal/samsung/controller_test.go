package samsung

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/galaxytty/galaxytty/internal/domain"
)

type controllerDevice struct {
	calls      [][]string
	stdinCalls []string
	output     []byte
	outputs    [][]byte
	err        error
}

func (d *controllerDevice) ShellStdin(_ context.Context, command string) ([]byte, error) {
	d.stdinCalls = append(d.stdinCalls, command)
	return d.output, d.err
}

func (d *controllerDevice) State(context.Context) (domain.DeviceState, error) {
	return domain.DeviceConnected, nil
}

func (d *controllerDevice) Shell(_ context.Context, args ...string) ([]byte, error) {
	d.calls = append(d.calls, append([]string(nil), args...))
	if len(d.outputs) > 0 {
		output := d.outputs[0]
		d.outputs = d.outputs[1:]
		return output, d.err
	}
	return d.output, d.err
}

func TestControllerUsesDisplaySpecificPublicADBCommands(t *testing.T) {
	device := &controllerDevice{outputs: [][]byte{
		[]byte("com.samsung.android.messaging\n"),
	}}
	controller, err := NewController(device, DefaultLayout())
	if err != nil {
		t.Fatal(err)
	}
	display := domain.VirtualDisplay{AndroidDisplayID: 18, Width: 1080, Height: 1920}
	if err := controller.EnsureDefaultSMSHandler(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := controller.ShowHome(context.Background(), display); err != nil {
		t.Fatal(err)
	}
	if err := controller.OpenConversation(context.Background(), display, "01012345678"); err != nil {
		t.Fatal(err)
	}
	if err := controller.OpenConversationWithBody(context.Background(), display, "+82 10-1234-5678", "It's 한글 😀"); err != nil {
		t.Fatal(err)
	}
	if err := controller.FocusComposer(context.Background(), display); err != nil {
		t.Fatal(err)
	}
	if err := controller.ClearComposer(context.Background(), display); err != nil {
		t.Fatal(err)
	}
	if err := controller.Paste(context.Background(), display); err != nil {
		t.Fatal(err)
	}
	if err := controller.TapSend(context.Background(), display); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"cmd", "role", "get-role-holders", "android.app.role.SMS"},
		{"input", "-d", "18", "keyevent", "3"},
		{"input", "-d", "18", "tap", "500", "1800"},
		{"input", "-d", "18", "keycombination", "113", "29"},
		{"input", "-d", "18", "keyevent", "67"},
		{"input", "-d", "18", "keyevent", "279"},
		{"input", "-d", "18", "tap", "1004", "955"},
	}
	if !reflect.DeepEqual(device.calls, want) {
		t.Fatalf("calls=%q want=%q", device.calls, want)
	}
	wantStdin := []string{
		"am start --display 18 -a android.intent.action.SENDTO -d 'smsto:01012345678' -p com.samsung.android.messaging\n",
		`am start --display 18 -a android.intent.action.SENDTO -d 'smsto:01012345678' -p com.samsung.android.messaging --es sms_body 'It'"'"'s 한글 😀'` + "\n",
	}
	if !reflect.DeepEqual(device.stdinCalls, wantStdin) {
		t.Fatalf("stdin calls=%q want=%q", device.stdinCalls, wantStdin)
	}
}

func TestRemoteShellQuotePreservesArbitraryTextWithoutInterpolation(t *testing.T) {
	got, err := quoteRemoteShellArg("line 1\n$HOME `cmd` 'quoted' 한글 😀")
	if err != nil {
		t.Fatal(err)
	}
	want := `'line 1
$HOME ` + "`cmd`" + ` '"'"'quoted'"'"' 한글 😀'`
	if got != want {
		t.Fatalf("quoted value mismatch: %q", got)
	}
	if _, err := quoteRemoteShellArg("bad\x00value"); err == nil {
		t.Fatal("expected NUL rejection")
	}
}

func TestControllerRejectsInvalidDisplayAndLayoutBeforeADB(t *testing.T) {
	device := &controllerDevice{}
	invalid := DefaultLayout()
	invalid.Send.X = invalid.Width
	if _, err := NewController(device, invalid); err == nil {
		t.Fatal("expected invalid layout error")
	}
	controller, err := NewController(device, DefaultLayout())
	if err != nil {
		t.Fatal(err)
	}
	if err := controller.Paste(context.Background(), domain.VirtualDisplay{}); err == nil {
		t.Fatal("expected invalid display error")
	}
	if len(device.calls) != 0 {
		t.Fatalf("unexpected ADB calls: %q", device.calls)
	}
}

func TestControllerWrapsSafeOperationErrors(t *testing.T) {
	const phone = "01012345678"
	const body = "private message body"
	for _, tc := range []struct {
		name string
		run  func(*Controller) error
		want error
	}{
		{"open", func(c *Controller) error {
			return c.OpenConversation(context.Background(), domain.VirtualDisplay{AndroidDisplayID: 18}, phone)
		}, domain.ErrConversationOpen},
		{"open-body", func(c *Controller) error {
			return c.OpenConversationWithBody(context.Background(), domain.VirtualDisplay{AndroidDisplayID: 18}, phone, body)
		}, domain.ErrConversationOpen},
		{"composer", func(c *Controller) error {
			return c.FocusComposer(context.Background(), domain.VirtualDisplay{AndroidDisplayID: 18})
		}, domain.ErrComposerTap},
		{"clear", func(c *Controller) error {
			return c.ClearComposer(context.Background(), domain.VirtualDisplay{AndroidDisplayID: 18})
		}, domain.ErrComposerClear},
		{"paste", func(c *Controller) error {
			return c.Paste(context.Background(), domain.VirtualDisplay{AndroidDisplayID: 18})
		}, domain.ErrClipboardPaste},
		{"send", func(c *Controller) error {
			return c.TapSend(context.Background(), domain.VirtualDisplay{AndroidDisplayID: 18})
		}, domain.ErrSendTap},
		{"home", func(c *Controller) error {
			return c.ShowHome(context.Background(), domain.VirtualDisplay{AndroidDisplayID: 18})
		}, domain.ErrClipboardSync},
	} {
		t.Run(tc.name, func(t *testing.T) {
			device := &controllerDevice{err: errors.New("synthetic adb failure")}
			controller, err := NewController(device, DefaultLayout())
			if err != nil {
				t.Fatal(err)
			}
			err = tc.run(controller)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err=%v want=%v", err, tc.want)
			}
			if strings.Contains(err.Error(), phone) || strings.Contains(err.Error(), body) {
				t.Fatalf("error exposes private send data: %q", err)
			}
		})
	}
}

func TestOpenConversationRejectsActivityManagerErrorOutput(t *testing.T) {
	device := &controllerDevice{output: []byte("Error: Activity not started, unable to resolve Intent")}
	controller, err := NewController(device, DefaultLayout())
	if err != nil {
		t.Fatal(err)
	}
	err = controller.OpenConversation(context.Background(), domain.VirtualDisplay{AndroidDisplayID: 18}, "01012345678")
	if !errors.Is(err, domain.ErrConversationOpen) {
		t.Fatalf("err=%v", err)
	}
}
