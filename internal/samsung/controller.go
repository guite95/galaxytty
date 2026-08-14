package samsung

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/galaxytty/galaxytty/internal/domain"
)

const pasteKeyCode = "279"

type Controller struct {
	device domain.Device
	layout Layout
}

func NewController(device domain.Device, layout Layout) (*Controller, error) {
	if device == nil {
		return nil, errors.New("Samsung controller device is required")
	}
	if err := layout.Validate(); err != nil {
		return nil, err
	}
	return &Controller{device: device, layout: layout}, nil
}

func (c *Controller) OpenConversation(ctx context.Context, display domain.VirtualDisplay, phone string) error {
	if err := validateDisplay(display); err != nil {
		return err
	}
	phone = strings.TrimSpace(phone)
	if phone == "" {
		return safeControllerError{kind: domain.ErrConversationOpen, cause: errors.New("recipient is required")}
	}
	return c.run(ctx, domain.ErrConversationOpen,
		"am", "start",
		"--display", displayID(display),
		"-a", "android.intent.action.SENDTO",
		"-d", "smsto:"+phone,
	)
}

func (c *Controller) FocusComposer(ctx context.Context, display domain.VirtualDisplay) error {
	if err := validateDisplay(display); err != nil {
		return err
	}
	return c.run(ctx, domain.ErrComposerTap,
		"input", "-d", displayID(display), "tap",
		strconv.Itoa(c.layout.Composer.X), strconv.Itoa(c.layout.Composer.Y),
	)
}

func (c *Controller) Paste(ctx context.Context, display domain.VirtualDisplay) error {
	if err := validateDisplay(display); err != nil {
		return err
	}
	return c.run(ctx, domain.ErrClipboardPaste,
		"input", "-d", displayID(display), "keyevent", pasteKeyCode,
	)
}

func (c *Controller) TapSend(ctx context.Context, display domain.VirtualDisplay) error {
	if err := validateDisplay(display); err != nil {
		return err
	}
	return c.run(ctx, domain.ErrSendTap,
		"input", "-d", displayID(display), "tap",
		strconv.Itoa(c.layout.Send.X), strconv.Itoa(c.layout.Send.Y),
	)
}

func (c *Controller) run(ctx context.Context, kind error, args ...string) error {
	output, err := c.device.Shell(ctx, args...)
	if err != nil {
		return safeControllerError{kind: kind, cause: err}
	}
	lower := strings.ToLower(strings.TrimSpace(string(output)))
	if strings.HasPrefix(lower, "error:") || strings.Contains(lower, "exception") {
		return safeControllerError{kind: kind, cause: errors.New("Android command reported an error")}
	}
	return nil
}

func validateDisplay(display domain.VirtualDisplay) error {
	if display.AndroidDisplayID <= 0 {
		return domain.ErrVirtualDisplayIDNotFound
	}
	return nil
}

func displayID(display domain.VirtualDisplay) string {
	return strconv.FormatInt(display.AndroidDisplayID, 10)
}

type safeControllerError struct {
	kind  error
	cause error
}

func (e safeControllerError) Error() string { return e.kind.Error() }
func (e safeControllerError) Unwrap() []error {
	return []error{e.kind, e.cause}
}

var _ domain.ConversationController = (*Controller)(nil)
