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
	calls  [][]string
	output []byte
	err    error
}

func (d *controllerDevice) State(context.Context) (domain.DeviceState, error) {
	return domain.DeviceConnected, nil
}

func (d *controllerDevice) Shell(_ context.Context, args ...string) ([]byte, error) {
	d.calls = append(d.calls, append([]string(nil), args...))
	return d.output, d.err
}

func TestControllerUsesDisplaySpecificPublicADBCommands(t *testing.T) {
	device := &controllerDevice{}
	controller, err := NewController(device, DefaultLayout())
	if err != nil {
		t.Fatal(err)
	}
	display := domain.VirtualDisplay{AndroidDisplayID: 18, Width: 1080, Height: 1920}
	if err := controller.OpenConversation(context.Background(), display, "01012345678"); err != nil {
		t.Fatal(err)
	}
	if err := controller.FocusComposer(context.Background(), display); err != nil {
		t.Fatal(err)
	}
	if err := controller.Paste(context.Background(), display); err != nil {
		t.Fatal(err)
	}
	if err := controller.TapSend(context.Background(), display); err != nil {
		t.Fatal(err)
	}

	want := [][]string{
		{"am", "start", "--display", "18", "-a", "android.intent.action.SENDTO", "-d", "smsto:01012345678"},
		{"input", "-d", "18", "tap", "500", "1800"},
		{"input", "-d", "18", "keyevent", "279"},
		{"input", "-d", "18", "tap", "1004", "1273"},
	}
	if !reflect.DeepEqual(device.calls, want) {
		t.Fatalf("calls=%q want=%q", device.calls, want)
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
	for _, tc := range []struct {
		name string
		run  func(*Controller) error
		want error
	}{
		{"open", func(c *Controller) error {
			return c.OpenConversation(context.Background(), domain.VirtualDisplay{AndroidDisplayID: 18}, phone)
		}, domain.ErrConversationOpen},
		{"composer", func(c *Controller) error {
			return c.FocusComposer(context.Background(), domain.VirtualDisplay{AndroidDisplayID: 18})
		}, domain.ErrComposerTap},
		{"paste", func(c *Controller) error {
			return c.Paste(context.Background(), domain.VirtualDisplay{AndroidDisplayID: 18})
		}, domain.ErrClipboardPaste},
		{"send", func(c *Controller) error {
			return c.TapSend(context.Background(), domain.VirtualDisplay{AndroidDisplayID: 18})
		}, domain.ErrSendTap},
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
			if strings.Contains(err.Error(), phone) {
				t.Fatalf("error exposes recipient: %q", err)
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
