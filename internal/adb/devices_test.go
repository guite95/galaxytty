package adb

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/galaxytty/galaxytty/internal/domain"
)

func TestParseDevicesClassifiesTransportMetadata(t *testing.T) {
	input := `List of devices attached
USB123 device usb:123 product:a37 model:SM_A376N transport_id:1
adb-USB123-token._adb-tls-connect._tcp device model:SM_A376N transport_id:2
192.0.2.10:37123 offline transport_id:3
OTHER unauthorized transport_id:4
local:transport device transport_id:5
`
	devices, err := ParseDevices(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 5 {
		t.Fatalf("len=%d", len(devices))
	}
	if devices[0].Connection != domain.ConnectionUSB || devices[0].Metadata["usb"] != "123" {
		t.Fatalf("usb=%+v", devices[0])
	}
	if devices[1].Connection != domain.ConnectionWireless || devices[2].Connection != domain.ConnectionWireless {
		t.Fatalf("wireless=%+v %+v", devices[1], devices[2])
	}
	if devices[2].State != "offline" || devices[3].State != "unauthorized" {
		t.Fatalf("states=%+v %+v", devices[2], devices[3])
	}
	if devices[4].Connection != "" {
		t.Fatalf("fragile colon classification: %+v", devices[4])
	}
}

func TestClientDevicesUsesLongListingAndParsesIt(t *testing.T) {
	runner := &recordingRunner{stdout: []byte("List of devices attached\nUSB123 device usb:123 transport_id:1\n")}
	client := newClient("/synthetic/adb", time.Second, runner)
	devices, err := client.Devices(context.Background())
	if err != nil || len(devices) != 1 || devices[0].Target != "USB123" {
		t.Fatalf("devices=%+v err=%v", devices, err)
	}
	if !reflect.DeepEqual(runner.args, []string{"devices", "-l"}) {
		t.Fatalf("args=%q", runner.args)
	}
}

func TestMDNSServicesParsesTargetsAndMapsUnavailable(t *testing.T) {
	runner := &recordingRunner{stdout: []byte("List of discovered mdns services\nsynthetic._adb-tls-connect._tcp 192.0.2.20:37123\n")}
	client := newClient("/synthetic/adb", time.Second, runner)
	services, err := client.MDNSServices(context.Background())
	if err != nil || len(services) != 1 || services[0].Name != "synthetic._adb-tls-connect._tcp" || services[0].Target != "192.0.2.20:37123" {
		t.Fatalf("services=%+v err=%v", services, err)
	}
	if !reflect.DeepEqual(runner.args, []string{"mdns", "services"}) {
		t.Fatalf("args=%q", runner.args)
	}

	runner = &recordingRunner{stderr: []byte("unknown command mdns"), err: errors.New("exit status 1")}
	client = newClient("/synthetic/adb", time.Second, runner)
	if _, err := client.MDNSServices(context.Background()); !errors.Is(err, ErrWirelessDiscoveryUnavailable) {
		t.Fatalf("err=%v", err)
	}
}

func TestParseDevicesRejectsMalformedRecord(t *testing.T) {
	if _, err := ParseDevices("List of devices attached\nmissing-state\n"); err == nil {
		t.Fatal("expected malformed record error")
	}
}
