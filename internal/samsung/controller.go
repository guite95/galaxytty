package samsung

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/galaxytty/galaxytty/internal/domain"
)

const (
	pasteKeyCode  = "279"
	deleteKeyCode = "67"
	ctrlKeyCode   = "113"
	aKeyCode      = "29"
)

const (
	wakeKeyCode  = "224"
	sleepKeyCode = "223"
)

type Controller struct {
	device     domain.Device
	stdinShell interface {
		ShellStdin(context.Context, string) ([]byte, error)
	}
	layout Layout
}

func NewController(device domain.Device, layout Layout) (*Controller, error) {
	if device == nil {
		return nil, errors.New("Samsung controller device is required")
	}
	stdinShell, ok := device.(interface {
		ShellStdin(context.Context, string) ([]byte, error)
	})
	if !ok {
		return nil, errors.New("Samsung controller requires remote shell stdin")
	}
	if err := layout.Validate(); err != nil {
		return nil, err
	}
	return &Controller{device: device, stdinShell: stdinShell, layout: layout}, nil
}

func (c *Controller) OpenConversation(ctx context.Context, display domain.VirtualDisplay, phone string) error {
	if err := validateDisplay(display); err != nil {
		return err
	}
	phone = domain.NormalizePhone(phone)
	if phone == "" {
		return safeControllerError{kind: domain.ErrConversationOpen, cause: errors.New("recipient is required")}
	}
	command, err := sendToCommand(display, phone, "")
	if err != nil {
		return safeControllerError{kind: domain.ErrConversationOpen, cause: err}
	}
	return c.runStdin(ctx, domain.ErrConversationOpen, command)
}

func (c *Controller) EnsureDefaultSMSHandler(ctx context.Context) error {
	return ensureDefaultSMSHandler(ctx, c.device)
}

func (c *Controller) OpenConversationWithBody(ctx context.Context, display domain.VirtualDisplay, phone, body string) error {
	if err := validateDisplay(display); err != nil {
		return err
	}
	phone = domain.NormalizePhone(phone)
	if phone == "" || strings.TrimSpace(body) == "" {
		return safeControllerError{kind: domain.ErrConversationOpen, cause: errors.New("recipient and message body are required")}
	}
	command, err := sendToCommand(display, phone, body)
	if err != nil {
		return safeControllerError{kind: domain.ErrConversationOpen, cause: err}
	}
	return c.runStdin(ctx, domain.ErrConversationOpen, command)
}

func (c *Controller) MainDisplayOff(ctx context.Context) (bool, error) {
	output, err := c.device.Shell(ctx, "dumpsys", "display")
	if err != nil {
		return false, safeControllerError{kind: domain.ErrDisplayPower, cause: err}
	}
	lower := strings.ToLower(strings.TrimSpace(string(output)))
	if strings.HasPrefix(lower, "error:") || strings.Contains(lower, "exception") {
		return false, safeControllerError{kind: domain.ErrDisplayPower, cause: errors.New("Android command reported an error")}
	}
	off, ok := parseDisplayZeroOff(string(output))
	if !ok {
		return false, safeControllerError{kind: domain.ErrDisplayPower, cause: errors.New("main display state was not found")}
	}
	return off, nil
}

func (c *Controller) WakeVirtualDisplay(ctx context.Context, display domain.VirtualDisplay) error {
	if err := validateDisplay(display); err != nil {
		return err
	}
	return c.run(ctx, domain.ErrDisplayPower,
		"input", "-d", displayID(display), "keyevent", wakeKeyCode,
	)
}

func (c *Controller) ShowHome(ctx context.Context, display domain.VirtualDisplay) error {
	if err := validateDisplay(display); err != nil {
		return err
	}
	return c.run(ctx, domain.ErrClipboardSync,
		"input", "-d", displayID(display), "keyevent", "3",
	)
}

func (c *Controller) SleepMainDisplay(ctx context.Context) error {
	return c.run(ctx, domain.ErrDisplayPower,
		"input", "-d", "0", "keyevent", sleepKeyCode,
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

func (c *Controller) ClearComposer(ctx context.Context, display domain.VirtualDisplay) error {
	if err := validateDisplay(display); err != nil {
		return err
	}
	if err := c.run(ctx, domain.ErrComposerClear,
		"input", "-d", displayID(display), "keycombination", ctrlKeyCode, aKeyCode,
	); err != nil {
		return err
	}
	return c.run(ctx, domain.ErrComposerClear,
		"input", "-d", displayID(display), "keyevent", deleteKeyCode,
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

func (c *Controller) runStdin(ctx context.Context, kind error, command string) error {
	output, err := c.stdinShell.ShellStdin(ctx, command+"\n")
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

func quoteRemoteShellArg(value string) (string, error) {
	if strings.IndexByte(value, 0) >= 0 {
		return "", errors.New("NUL bytes are not supported")
	}
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'", nil
}

func sendToCommand(display domain.VirtualDisplay, phone, body string) (string, error) {
	uri, err := quoteRemoteShellArg("smsto:" + phone)
	if err != nil {
		return "", err
	}
	command := "am start --display " + displayID(display) +
		" -a android.intent.action.SENDTO -d " + uri + " -p " + MessagesPackage
	if body == "" {
		return command, nil
	}
	quotedBody, err := quoteRemoteShellArg(body)
	if err != nil {
		return "", err
	}
	return command + " --es sms_body " + quotedBody, nil
}

func parseDisplayZeroOff(output string) (off, ok bool) {
	inDisplayZero := false
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Display Id=") {
			inDisplayZero = line == "Display Id=0"
			continue
		}
		if !inDisplayZero || !strings.HasPrefix(line, "Display State=") {
			continue
		}
		state := strings.TrimSpace(strings.TrimPrefix(line, "Display State="))
		switch state {
		case "OFF", "DOZE", "DOZE_SUSPEND":
			return true, true
		case "ON", "VR", "ON_SUSPEND":
			return false, true
		default:
			return false, false
		}
	}
	return false, false
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
