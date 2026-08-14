package adb

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/galaxytty/galaxytty/internal/domain"
)

type Device struct {
	Target     string
	State      string
	Metadata   map[string]string
	Connection domain.ConnectionKind
}

type MDNSService struct {
	Name   string
	Target string
}

func ParseDevices(input string) ([]Device, error) {
	var devices []Device
	for _, rawLine := range strings.Split(strings.ReplaceAll(input, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "List of devices attached") || strings.HasPrefix(line, "*") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return nil, fmt.Errorf("malformed adb devices record")
		}
		device := Device{Target: fields[0], State: fields[1], Metadata: map[string]string{}}
		for _, field := range fields[2:] {
			parts := strings.SplitN(field, ":", 2)
			if len(parts) == 2 {
				device.Metadata[parts[0]] = parts[1]
			}
		}
		_, hasUSB := device.Metadata["usb"]
		switch {
		case hasUSB:
			device.Connection = domain.ConnectionUSB
		case strings.HasSuffix(device.Target, "._adb-tls-connect._tcp"):
			device.Connection = domain.ConnectionWireless
		case isNetworkTarget(device.Target):
			device.Connection = domain.ConnectionWireless
		}
		devices = append(devices, device)
	}
	return devices, nil
}

func isNetworkTarget(target string) bool {
	_, port, err := net.SplitHostPort(target)
	if err != nil {
		return false
	}
	n, err := strconv.Atoi(port)
	return err == nil && n > 0 && n <= 65535
}

func (c *Client) Devices(ctx context.Context) ([]Device, error) {
	stdout, err := c.run(ctx, "devices", "-l")
	if err != nil {
		return nil, err
	}
	return ParseDevices(string(stdout))
}

func (c *Client) MDNSServices(ctx context.Context) ([]MDNSService, error) {
	stdout, err := c.run(ctx, "mdns", "services")
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrWirelessDiscoveryUnavailable, err)
	}
	var services []MDNSService
	for _, rawLine := range strings.Split(string(stdout), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "List of discovered mdns services") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return nil, fmt.Errorf("%w: malformed mDNS service record", ErrWirelessDiscoveryUnavailable)
		}
		services = append(services, MDNSService{Name: fields[0], Target: fields[len(fields)-1]})
	}
	return services, nil
}
