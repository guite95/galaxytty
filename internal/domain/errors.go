package domain

import "errors"

var ErrSendingNotImplemented = errors.New("sending is not available yet (Phase 3-A read-only mode)")

var (
	ErrADBNotFound                  = errors.New("adb executable not found")
	ErrNoDevices                    = errors.New("no authorized Galaxy found")
	ErrUnauthorized                 = errors.New("adb device unauthorized")
	ErrOffline                      = errors.New("adb device offline")
	ErrMultipleDevices              = errors.New("multiple eligible Galaxy devices found")
	ErrSamsungMessagesNotInstalled  = errors.New("Samsung Messages is not installed")
	ErrWirelessDiscoveryUnavailable = errors.New("wireless ADB discovery is unavailable")
	ErrProviderPermissionDenied     = errors.New("Android provider permission denied")
	ErrProviderOutput               = errors.New("malformed Android provider output")
	ErrScrcpyNotFound               = errors.New("scrcpy executable not found")
	ErrScrcpyStartup                = errors.New("scrcpy startup failed")
	ErrVirtualDisplayIDNotFound     = errors.New("virtual display ID not found")
	ErrScrcpyExited                 = errors.New("scrcpy exited unexpectedly")
	ErrConversationOpen             = errors.New("Samsung Messages conversation open failed")
	ErrComposerTap                  = errors.New("Samsung Messages composer focus failed")
	ErrClipboardRead                = errors.New("clipboard read failed")
	ErrClipboardSet                 = errors.New("clipboard set failed")
	ErrClipboardPaste               = errors.New("clipboard paste failed")
	ErrSendTap                      = errors.New("Samsung Messages send tap failed")
	ErrSendVerificationTimeout      = errors.New("send verification timed out")
)
