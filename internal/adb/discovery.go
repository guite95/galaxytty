package adb

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/galaxytty/galaxytty/internal/domain"
)

const samsungMessagesPackage = "com.samsung.android.messaging"

type Backend interface {
	Devices(context.Context) ([]Device, error)
	Shell(context.Context, string, ...string) ([]byte, error)
}

type SelectionOptions struct {
	PreferUSB bool
	Target    string
	Package   string
}

type inspectedDevice struct {
	device         Device
	model          string
	hardwareSerial string
}

type Target struct {
	backend Backend
	info    domain.DeviceInfo

	mu    sync.RWMutex
	state domain.DeviceState
}

func Discover(ctx context.Context, backend Backend, options SelectionOptions) (*Target, error) {
	devices, err := backend.Devices(ctx)
	if err != nil {
		return nil, err
	}
	if options.Target != "" {
		var selected []Device
		for _, device := range devices {
			if device.Target == options.Target {
				selected = append(selected, device)
				break
			}
		}
		devices = selected
	}
	if len(devices) == 0 {
		return nil, ErrNoDevices
	}

	var authorized []Device
	unauthorized, offline := false, false
	for _, device := range devices {
		switch device.State {
		case "device":
			authorized = append(authorized, device)
		case "unauthorized":
			unauthorized = true
		case "offline":
			offline = true
		}
	}
	if len(authorized) == 0 {
		switch {
		case unauthorized:
			return nil, ErrUnauthorized
		case offline:
			return nil, ErrOffline
		default:
			return nil, ErrNoDevices
		}
	}

	packageName := options.Package
	if packageName == "" {
		packageName = samsungMessagesPackage
	}
	groups := map[string][]inspectedDevice{}
	missingPackage := false
	for _, device := range authorized {
		manufacturer, err := shellText(ctx, backend, device.Target, "getprop", "ro.product.manufacturer")
		if err != nil {
			return nil, fmt.Errorf("inspect manufacturer for %s: %w", device.Target, err)
		}
		if !strings.EqualFold(manufacturer, "samsung") {
			continue
		}
		model, err := shellText(ctx, backend, device.Target, "getprop", "ro.product.model")
		if err != nil {
			return nil, fmt.Errorf("inspect model for %s: %w", device.Target, err)
		}
		hardwareSerial, err := shellText(ctx, backend, device.Target, "getprop", "ro.serialno")
		if err != nil {
			return nil, fmt.Errorf("inspect serial for %s: %w", device.Target, err)
		}
		packagePath, err := shellText(ctx, backend, device.Target, "pm", "path", packageName)
		if err != nil {
			return nil, fmt.Errorf("inspect Samsung Messages for %s: %w", device.Target, err)
		}
		if packagePath == "" {
			missingPackage = true
			continue
		}
		if hardwareSerial == "" {
			hardwareSerial = device.Target
		}
		groups[hardwareSerial] = append(groups[hardwareSerial], inspectedDevice{device: device, model: model, hardwareSerial: hardwareSerial})
	}
	if len(groups) == 0 {
		if missingPackage {
			return nil, ErrSamsungMessagesNotInstalled
		}
		return nil, ErrNoDevices
	}
	if len(groups) > 1 {
		return nil, ErrMultipleDevices
	}

	var endpoints []inspectedDevice
	for _, group := range groups {
		endpoints = group
	}
	selected := chooseEndpoint(endpoints, options.PreferUSB)
	return &Target{
		backend: backend,
		info: domain.DeviceInfo{
			Serial:     selected.device.Target,
			Model:      selected.model,
			State:      domain.DeviceConnected,
			Connection: selected.device.Connection,
		},
		state: domain.DeviceConnected,
	}, nil
}

func shellText(ctx context.Context, backend Backend, target string, args ...string) (string, error) {
	output, err := backend.Shell(ctx, target, args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func chooseEndpoint(endpoints []inspectedDevice, preferUSB bool) inspectedDevice {
	preferred := domain.ConnectionWireless
	if preferUSB {
		preferred = domain.ConnectionUSB
	}
	for _, endpoint := range endpoints {
		if endpoint.device.Connection == preferred {
			return endpoint
		}
	}
	return endpoints[0]
}

func (t *Target) Info() domain.DeviceInfo {
	t.mu.RLock()
	defer t.mu.RUnlock()
	info := t.info
	info.State = t.state
	return info
}

func (t *Target) State(context.Context) (domain.DeviceState, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.state, nil
}

func (t *Target) Shell(ctx context.Context, args ...string) ([]byte, error) {
	output, err := t.backend.Shell(ctx, t.info.Serial, args...)
	t.mu.Lock()
	defer t.mu.Unlock()
	switch {
	case err == nil:
		t.state = domain.DeviceConnected
	case errors.Is(err, ErrUnauthorized):
		t.state = domain.DeviceUnauthorized
	case errors.Is(err, ErrOffline), errors.Is(err, ErrNoDevices):
		t.state = domain.DeviceDisconnected
	}
	return output, err
}

func (t *Target) Status(context.Context) domain.ApplicationStatus {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.state != domain.DeviceConnected {
		return domain.ApplicationStatus{State: string(t.state), Connection: t.info.Connection, Label: "Offline"}
	}
	label := "USB"
	if t.info.Connection == domain.ConnectionWireless {
		label = "Wireless"
	}
	return domain.ApplicationStatus{State: string(t.state), Connection: t.info.Connection, Label: label}
}
