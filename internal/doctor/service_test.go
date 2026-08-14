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
	version           string
	versionErr        error
	devices           []adb.Device
	packageInstalled  bool
	defaultSMSHandler string
	providerOutput    map[string][]byte
	projectionOutput  map[string][]byte
	contentCalls      [][]string
	shellCalls        [][]string
	mdnsErr           error
}

func readyADB(connection domain.ConnectionKind) *fakeADB {
	target := "USB123"
	if connection == domain.ConnectionWireless {
		target = "adb-USB123-token._adb-tls-connect._tcp"
	}
	return &fakeADB{
		version:           "Android Debug Bridge version 1.0.41",
		devices:           []adb.Device{{Target: target, State: "device", Connection: connection}},
		packageInstalled:  true,
		defaultSMSHandler: "com.samsung.android.messaging",
		providerOutput:    map[string][]byte{},
		projectionOutput:  map[string][]byte{},
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
	f.shellCalls = append(f.shellCalls, append([]string(nil), args...))
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
	case "cmd role get-role-holders android.app.role.SMS":
		if f.defaultSMSHandler == "" {
			return []byte("\n"), nil
		}
		return []byte(f.defaultSMSHandler + "\n"), nil
	}
	if len(args) > 0 && args[0] == "content" {
		f.contentCalls = append(f.contentCalls, append([]string(nil), args...))
		uri := argAfter(args, "--uri")
		projection := argAfter(args, "--projection")
		if output, ok := f.projectionOutput[uri+"|"+projection]; ok {
			return output, nil
		}
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
				LookPath: func(name string) (string, error) { return "/synthetic/" + name, nil },
			}
			report := service.Run(context.Background(), config.Default(), "")
			if !report.Ready || !report.ReadReady || !report.SendReady || !strings.Contains(report.Summary, "Read: ready") || !strings.Contains(report.Summary, "Send: ready") {
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
			for _, call := range backend.shellCalls {
				if len(call) > 0 && (call[0] == "am" || call[0] == "input") {
					t.Fatalf("doctor executed mutating call %q", call)
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

func TestDoctorReportsRCSStandardExtensionVisibilityPerColumn(t *testing.T) {
	backend := readyADB(domain.ConnectionUSB)
	backend.projectionOutput["content://sms|_id:chat_type"] = []byte("[ERROR] no such column: chat_type\n")
	report := (Service{ADB: backend}).Run(context.Background(), config.Default(), "")
	if !report.ReadReady || !report.SendReady {
		t.Fatalf("report=%+v", report)
	}
	check := findCheck(report, "RCS standard extension visibility")
	for _, want := range []string{"teleservice_id", "app_id", "correlation_tag", "chat_type unavailable", "mapping unsupported"} {
		if !strings.Contains(check.Detail, want) {
			t.Fatalf("RCS check missing %q: %+v", want, check)
		}
	}
	if check.State != Info {
		t.Fatalf("RCS check=%+v", check)
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

func TestDoctorKeepsReadReadyWhenSendToolsAreMissing(t *testing.T) {
	backend := readyADB(domain.ConnectionUSB)
	backend.mdnsErr = errors.New("mdns unavailable")
	report := (Service{
		ADB: backend,
		InspectScrcpy: func(context.Context, string) (scrcpy.VersionInfo, error) {
			return scrcpy.VersionInfo{}, errors.New("scrcpy missing")
		},
		LookPath: func(name string) (string, error) { return "/synthetic/" + name, nil },
	}).Run(context.Background(), config.Default(), "")
	if !report.Ready || !report.ReadReady || report.SendReady || findCheck(report, "Wireless discovery").State != Info || findCheck(report, "scrcpy").State != Fail {
		t.Fatalf("report=%+v", report)
	}
}

func TestDoctorMissingClipboardKeepsIntentBodySendReady(t *testing.T) {
	for _, missing := range []string{"pbcopy", "pbpaste", "osascript"} {
		t.Run(missing, func(t *testing.T) {
			backend := readyADB(domain.ConnectionUSB)
			report := (Service{
				ADB: backend,
				InspectScrcpy: func(context.Context, string) (scrcpy.VersionInfo, error) {
					return scrcpy.VersionInfo{Path: "/synthetic/scrcpy", Version: "4.1"}, nil
				},
				LookPath: func(name string) (string, error) {
					if name == missing {
						return "", errors.New("missing")
					}
					return "/synthetic/" + name, nil
				},
			}).Run(context.Background(), config.Default(), "")
			if !report.ReadReady || !report.SendReady || findCheck(report, "macOS clipboard compatibility").State != Info || !strings.Contains(report.Summary, "Send: ready") {
				t.Fatalf("report=%+v", report)
			}
		})
	}
}

func TestDoctorNonDefaultSamsungRoleKeepsReadReadyAndBlocksSend(t *testing.T) {
	backend := readyADB(domain.ConnectionUSB)
	backend.defaultSMSHandler = "example.other"
	report := (Service{
		ADB: backend,
		InspectScrcpy: func(context.Context, string) (scrcpy.VersionInfo, error) {
			return scrcpy.VersionInfo{Path: "/synthetic/scrcpy", Version: "4.1"}, nil
		},
		LookPath: func(name string) (string, error) { return "/synthetic/" + name, nil },
	}).Run(context.Background(), config.Default(), "")
	if !report.ReadReady || report.SendReady {
		t.Fatalf("report=%+v", report)
	}
	check := findCheck(report, "Samsung Messages default SMS handler")
	if check.State != Fail || !strings.Contains(check.Detail, "not default") {
		t.Fatalf("default role check=%+v", check)
	}
}

func TestDoctorClipboardModeRequiresCompatibilityTools(t *testing.T) {
	backend := readyADB(domain.ConnectionUSB)
	cfg := config.Default()
	cfg.Samsung.TextInputMode = domain.TextInputClipboard
	report := (Service{
		ADB: backend,
		InspectScrcpy: func(context.Context, string) (scrcpy.VersionInfo, error) {
			return scrcpy.VersionInfo{Path: "/synthetic/scrcpy", Version: "4.1"}, nil
		},
		LookPath: func(name string) (string, error) {
			if name == "osascript" {
				return "", errors.New("missing")
			}
			return "/synthetic/" + name, nil
		},
	}).Run(context.Background(), cfg, "")
	if !report.ReadReady || report.SendReady {
		t.Fatalf("report=%+v", report)
	}
	if check := findCheck(report, "Text input mode"); check.State != Pass || check.Detail != "clipboard" {
		t.Fatalf("mode check=%+v", check)
	}
	if check := findCheck(report, "macOS clipboard compatibility"); check.State != Fail {
		t.Fatalf("clipboard check=%+v", check)
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
