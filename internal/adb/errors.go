package adb

import "github.com/galaxytty/galaxytty/internal/domain"

var (
	ErrADBNotFound                  = domain.ErrADBNotFound
	ErrNoDevices                    = domain.ErrNoDevices
	ErrUnauthorized                 = domain.ErrUnauthorized
	ErrOffline                      = domain.ErrOffline
	ErrMultipleDevices              = domain.ErrMultipleDevices
	ErrSamsungMessagesNotInstalled  = domain.ErrSamsungMessagesNotInstalled
	ErrWirelessDiscoveryUnavailable = domain.ErrWirelessDiscoveryUnavailable
)
