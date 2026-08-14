package doctor

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/galaxytty/galaxytty/internal/adb"
	"github.com/galaxytty/galaxytty/internal/config"
	"github.com/galaxytty/galaxytty/internal/domain"
	"github.com/galaxytty/galaxytty/internal/scrcpy"
)

type fakeADB struct {
	version          string
	versionErr       error
	devices          []adb.Device
	packageInstalled bool
	providerOutput   map[string][]byte
	contentCalls     [][]string
	mdnsErr          error
}

func readyADB(connection domain.ConnectionKind) *fakeADB {
	target := "USB123"
	if connection == domain.ConnectionWireless {
		target = "adb-USB123-token._adb-tls-connect._tcp"
	}
	return &fakeADB{
		version:          "Android Debug Bridge version 1.0.41",
		devices:          []adb.Device{{Target: target, State: "device", Connection: connection}},
		packageInstalled: true,
		providerOutput:   map[string][]byte{},
	}
}

func (f *fakeADB) Version(context.Context) (string, error) { return f.version, f.versionErr }

func (f *fakeADB) Devices(context.Context) ([]adb.Device, error) {
	return append([]adb.Device(nil), f.devices...), nil
}

func (f *fakeADB) MDNSServices(context.Context) ([]adb.MDNSService, error) {
	if f.mdnsErr != nil {
		return nil, f.mdnsErr
	}
	return nil, nil
}

func (f *fakeADB) Shell(_ context.Context, target string, args ...string) ([]byte, error) {
	if len(f.devices) > 0 && target != f.devices[0].Target {
		return nil, fmt.Errorf("unexpected target %q", target)
	}
	key := strings.Join(args, " ")
	switch key {
	case "getprop ro.product.manufacturer":
		return []byte("samsung\n"), nil
	case "getprop ro.product.model":
		return []byte("SM-A376N\n"), nil
	case "getprop ro.serialno":
		return []byte("HARDWARE123\n"), nil
	case "pm path com.samsung.android.messaging":
		if f.packageInstalled {
			return []byte("package:/synthetic/base.apk\n"), nil
		}
		return []byte("\n"), nil
	}
	if len(args) > 0 && args[0] == "content" {
		f.contentCalls = append(f.contentCalls, append([]string(nil), args...))
		uri := argAfter(args, "--uri")
		if output, ok := f.providerOutput[uri]; ok {
			return output, nil
		}
		return []byte("No result found.\n"), nil
	}
	return nil, fmt.Errorf("unexpected shell args %q", args)
}

func TestDoctorReadyUsesOnlyNonSensitiveProjections(t *testing.T) {
	for _, connection := range []domain.ConnectionKind{domain.ConnectionUSB, domain.ConnectionWireless} {
		t.Run(string(connection), func(t *testing.T) {
			backend := readyADB(connection)
			service := Service{
				ADB: backend,
				InspectScrcpy: func(context.Context, string) (scrcpy.VersionInfo, error) {
					return scrcpy.VersionInfo{Path: "/synthetic/scrcpy", Version: "4.1"}, nil
				},
			}
			report := service.Run(context.Background(), config.Default(), "")
			if !report.Ready || report.Summary != "Read mode ready. Sending not implemented yet." {
				t.Fatalf("report=%+v", report)
			}
			if check := findCheck(report, "Connection"); check.Detail != expectedConnectionLabel(connection) || check.State != Pass {
				t.Fatalf("connection check=%+v", check)
			}
			for _, call := range backend.contentCalls {
				projection := argAfter(call, "--projection")
				for _, forbidden := range []string{"body", "address", "display_name", "data1", "data4", "snippet"} {
					if strings.Contains(projection, forbidden) {
						t.Fatalf("sensitive doctor projection %q", projection)
					}
				}
			}
		})
	}
}

func TestDoctorRequiredProviderFailureBlocksReadiness(t *testing.T) {
	backend := readyADB(domain.ConnectionUSB)
	backend.providerOutput["content://sms"] = []byte("java.lang.SecurityException: Permission Denial\n")
	report := (Service{ADB: backend}).Run(context.Background(), config.Default(), "")
	if report.Ready || findCheck(report, "SMS Provider").State != Fail {
		t.Fatalf("report=%+v", report)
	}
}

func TestDoctorOptionalProviderFailureDoesNotBlockReadiness(t *testing.T) {
	backend := readyADB(domain.ConnectionUSB)
	backend.providerOutput["content://mms"] = []byte("[ERROR] synthetic optional failure\n")
	report := (Service{ADB: backend}).Run(context.Background(), config.Default(), "")
	if !report.Ready || findCheck(report, "MMS Provider").State != Info {
		t.Fatalf("report=%+v", report)
	}
}

func TestDoctorReportsADBAndSelectionFailures(t *testing.T) {
	for _, tc := range []struct {
		name    string
		backend *fakeADB
	}{
		{name: "adb missing", backend: &fakeADB{versionErr: adb.ErrADBNotFound}},
		{name: "unauthorized", backend: &fakeADB{version: "version", devices: []adb.Device{{Target: "USB123", State: "unauthorized"}}}},
		{name: "offline", backend: &fakeADB{version: "version", devices: []adb.Device{{Target: "USB123", State: "offline"}}}},
		{name: "package missing", backend: func() *fakeADB { b := readyADB(domain.ConnectionUSB); b.packageInstalled = false; return b }()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			report := (Service{ADB: tc.backend}).Run(context.Background(), config.Default(), "")
			if report.Ready || report.Summary == "" {
				t.Fatalf("report=%+v", report)
			}
		})
	}
}

func TestDoctorTreatsMDNSAndScrcpyAsInformational(t *testing.T) {
	backend := readyADB(domain.ConnectionUSB)
	backend.mdnsErr = errors.New("mdns unavailable")
	report := (Service{
		ADB: backend,
		InspectScrcpy: func(context.Context, string) (scrcpy.VersionInfo, error) {
			return scrcpy.VersionInfo{}, errors.New("scrcpy missing")
		},
	}).Run(context.Background(), config.Default(), "")
	if !report.Ready || findCheck(report, "Wireless discovery").State != Info || findCheck(report, "scrcpy").State != Info {
		t.Fatalf("report=%+v", report)
	}
}

func findCheck(report Report, name string) Check {
	for _, check := range report.Checks {
		if check.Name == name {
			return check
		}
	}
	return Check{}
}

func argAfter(args []string, name string) string {
	for i := range args {
		if args[i] == name && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func expectedConnectionLabel(connection domain.ConnectionKind) string {
	if connection == domain.ConnectionWireless {
		return "Wireless"
	}
	return "USB"
}
