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
)
