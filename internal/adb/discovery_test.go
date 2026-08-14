package adb

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/galaxytty/galaxytty/internal/domain"
)

type fakeBackend struct {
	devices  []Device
	props    map[string]map[string]string
	shellErr map[string]error
}

func (f *fakeBackend) Devices(context.Context) ([]Device, error) {
	return append([]Device(nil), f.devices...), nil
}

func (f *fakeBackend) Shell(_ context.Context, target string, args ...string) ([]byte, error) {
	key := strings.Join(args, " ")
	if err := f.shellErr[target+"|"+key]; err != nil {
		return nil, err
	}
	if value, ok := f.props[target][key]; ok {
		return []byte(value + "\n"), nil
	}
	return nil, fmt.Errorf("unexpected shell call target=%s args=%q", target, args)
}

func galaxyProps(serial string, installed bool) map[string]string {
	packagePath := ""
	if installed {
		packagePath = "package:/synthetic/base.apk"
	}
	return map[string]string{
		"getprop ro.product.manufacturer":       "samsung",
		"getprop ro.product.model":              "SM-A376N",
		"getprop ro.serialno":                   serial,
		"pm path com.samsung.android.messaging": packagePath,
	}
}

func sameGalaxyBackend() *fakeBackend {
	usb := Device{Target: "USB123", State: "device", Connection: domain.ConnectionUSB}
	wireless := Device{Target: "adb-USB123-token._adb-tls-connect._tcp", State: "device", Connection: domain.ConnectionWireless}
	return &fakeBackend{
		devices: []Device{usb, wireless},
		props: map[string]map[string]string{
			usb.Target:      galaxyProps("HARDWARE123", true),
			wireless.Target: galaxyProps("HARDWARE123", true),
		},
		shellErr: map[string]error{},
	}
}

func TestDiscoverDeduplicatesSamePhoneAndPrefersUSB(t *testing.T) {
	target, err := Discover(context.Background(), sameGalaxyBackend(), SelectionOptions{PreferUSB: true})
	if err != nil {
		t.Fatal(err)
	}
	if info := target.Info(); info.Serial != "USB123" || info.Connection != domain.ConnectionUSB || info.Model != "SM-A376N" {
		t.Fatalf("%+v", info)
	}
}

func TestDiscoverCanPreferWireless(t *testing.T) {
	target, err := Discover(context.Background(), sameGalaxyBackend(), SelectionOptions{PreferUSB: false})
	if err != nil {
		t.Fatal(err)
	}
	if info := target.Info(); info.Connection != domain.ConnectionWireless {
		t.Fatalf("%+v", info)
	}
}

func TestDiscoverHonorsExplicitTarget(t *testing.T) {
	backend := sameGalaxyBackend()
	selected := backend.devices[1].Target
	target, err := Discover(context.Background(), backend, SelectionOptions{PreferUSB: true, Target: selected})
	if err != nil {
		t.Fatal(err)
	}
	if info := target.Info(); info.Serial != selected || info.Connection != domain.ConnectionWireless {
		t.Fatalf("%+v", info)
	}
}

func TestDiscoverRejectsMultiplePhysicalGalaxies(t *testing.T) {
	backend := sameGalaxyBackend()
	backend.devices = append(backend.devices, Device{Target: "USB999", State: "device", Connection: domain.ConnectionUSB})
	backend.props["USB999"] = galaxyProps("HARDWARE999", true)
	_, err := Discover(context.Background(), backend, SelectionOptions{PreferUSB: true})
	if !errors.Is(err, ErrMultipleDevices) {
		t.Fatalf("err=%v", err)
	}
}

func TestDiscoverMapsUnavailableStates(t *testing.T) {
	for _, tc := range []struct {
		name    string
		devices []Device
		want    error
	}{
		{name: "none", want: ErrNoDevices},
		{name: "unauthorized", devices: []Device{{Target: "USB123", State: "unauthorized"}}, want: ErrUnauthorized},
		{name: "offline", devices: []Device{{Target: "USB123", State: "offline"}}, want: ErrOffline},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Discover(context.Background(), &fakeBackend{devices: tc.devices}, SelectionOptions{})
			if !errors.Is(err, tc.want) {
				t.Fatalf("err=%v want=%v", err, tc.want)
			}
		})
	}
}

func TestDiscoverRejectsMissingSamsungMessages(t *testing.T) {
	backend := &fakeBackend{
		devices: []Device{{Target: "USB123", State: "device", Connection: domain.ConnectionUSB}},
		props:   map[string]map[string]string{"USB123": galaxyProps("HARDWARE123", false)},
	}
	_, err := Discover(context.Background(), backend, SelectionOptions{})
	if !errors.Is(err, ErrSamsungMessagesNotInstalled) {
		t.Fatalf("err=%v", err)
	}
}

func TestDiscoverRejectsNonSamsungDevice(t *testing.T) {
	backend := &fakeBackend{
		devices: []Device{{Target: "OTHER", State: "device", Connection: domain.ConnectionUSB}},
		props:   map[string]map[string]string{"OTHER": galaxyProps("OTHER-HARDWARE", true)},
	}
	backend.props["OTHER"]["getprop ro.product.manufacturer"] = "other"
	_, err := Discover(context.Background(), backend, SelectionOptions{})
	if !errors.Is(err, ErrNoDevices) {
		t.Fatalf("err=%v", err)
	}
}

func TestTargetUpdatesCachedStatusAfterOfflineError(t *testing.T) {
	backend := sameGalaxyBackend()
	target, err := Discover(context.Background(), backend, SelectionOptions{PreferUSB: true})
	if err != nil {
		t.Fatal(err)
	}
	backend.shellErr["USB123|getprop ro.product.model"] = ErrOffline
	if _, err := target.Shell(context.Background(), "getprop", "ro.product.model"); !errors.Is(err, ErrOffline) {
		t.Fatalf("err=%v", err)
	}
	if got := target.Status(context.Background()); got.Label != "Offline" || got.State != string(domain.DeviceDisconnected) {
		t.Fatalf("%+v", got)
	}
}
