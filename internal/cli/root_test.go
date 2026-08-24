package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/galaxytty/galaxytty/internal/adb"
	"github.com/galaxytty/galaxytty/internal/app"
	"github.com/galaxytty/galaxytty/internal/bootstrap"
	"github.com/galaxytty/galaxytty/internal/config"
	"github.com/galaxytty/galaxytty/internal/doctor"
	"github.com/galaxytty/galaxytty/internal/domain"
	"github.com/galaxytty/galaxytty/internal/mock"
	"github.com/galaxytty/galaxytty/internal/remote"
)

func run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	e := Execute(context.Background(), strings.NewReader(""), &out, args)
	return out.String(), e
}
func TestHelp(t *testing.T) {
	v, e := run(t, "--help")
	if e != nil || !strings.Contains(v, "Usage:") {
		t.Fatal(v, e)
	}
}
func TestMockCommands(t *testing.T) {
	for _, a := range [][]string{{"conversations", "--mock"}, {"unread", "--mock"}, {"messages", "1", "--mock"}, {"doctor", "--mock"}, {"send", "--mock", "--to", "01012345678", "--text", "hello"}} {
		if _, e := run(t, a...); e != nil {
			t.Fatalf("%v: %v", a, e)
		}
	}
}

func TestMockDoctorReportsReadAndSendReady(t *testing.T) {
	output, err := run(t, "doctor", "--mock")
	if err != nil || !strings.Contains(output, "Read: ready") || !strings.Contains(output, "Send: ready") {
		t.Fatalf("output=%q err=%v", output, err)
	}
}
func TestJSONOnly(t *testing.T) {
	for _, a := range [][]string{
		{"conversations", "--mock", "--json"},
		{"unread", "--mock", "--json"},
		{"messages", "1", "--mock", "--json"},
		{"send", "--mock", "--to", "01012345678", "--text", "synthetic", "--json"},
	} {
		v, e := run(t, a...)
		if e != nil || !json.Valid([]byte(v)) {
			t.Fatalf("%v: %q %v", a, v, e)
		}
	}
}
func TestSendValidation(t *testing.T) {
	if _, e := run(t, "send", "--mock", "--to", "010"); e == nil {
		t.Fatal("expected text validation")
	}
}

func TestExecuteRejectsInvalidStartupConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	configDir := filepath.Join(dir, "galaxytty")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.toml"), []byte("unknown=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, "doctor", "--mock"); err == nil || !strings.Contains(err.Error(), "decode config") {
		t.Fatalf("err=%v", err)
	}
}

func TestRealModeUsesConfiguredDeviceAndCLIOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	configDir := filepath.Join(dir, "galaxytty")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.toml"), []byte("[connection]\ndevice='configured-target'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var gotConfig config.Config
	var gotSelector string
	deps := dependencies{
		real: func(_ context.Context, cfg config.Config, selector string) (*bootstrap.Runtime, error) {
			gotConfig, gotSelector = cfg, selector
			return bootstrap.Mock(cfg)
		},
	}
	var out bytes.Buffer
	if err := execute(context.Background(), strings.NewReader(""), &out, []string{"conversations"}, deps); err != nil {
		t.Fatal(err)
	}
	if gotConfig.Connection.Device != "configured-target" || gotSelector != "" {
		t.Fatalf("config device=%q selector=%q", gotConfig.Connection.Device, gotSelector)
	}
	out.Reset()
	if err := execute(context.Background(), strings.NewReader(""), &out, []string{"conversations", "--device", "cli-target"}, deps); err != nil {
		t.Fatal(err)
	}
	if gotSelector != "cli-target" {
		t.Fatalf("selector=%q", gotSelector)
	}
}

func TestHelperModeUsesAutomaticDiscoveryOrDebugAddress(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var addresses []string
	deps := dependencies{helper: func(_ context.Context, cfg config.Config, address string) (*bootstrap.Runtime, error) {
		addresses = append(addresses, address)
		return bootstrap.Mock(cfg)
	}}
	for _, args := range [][]string{
		{"conversations", "--helper"},
		{"conversations", "--helper", "--helper-address", "192.0.2.10:43127"},
	} {
		var out bytes.Buffer
		if err := execute(context.Background(), strings.NewReader(""), &out, args, deps); err != nil {
			t.Fatalf("args=%v err=%v", args, err)
		}
	}
	if len(addresses) != 2 || addresses[0] != "" || addresses[1] != "192.0.2.10:43127" {
		t.Fatalf("addresses=%v", addresses)
	}
}

func TestHelperFlagsRejectUnsafeOrAmbiguousCombinations(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	for _, args := range [][]string{
		{"conversations", "--mock", "--helper"},
		{"conversations", "--helper-address", "192.0.2.10:43127"},
	} {
		var out bytes.Buffer
		if err := execute(context.Background(), strings.NewReader(""), &out, args, dependencies{}); err == nil {
			t.Fatalf("args=%v expected error", args)
		}
	}
}

func TestHelperDoctorDoesNotInvokeMessagingCommands(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	called := false
	deps := dependencies{helper: func(_ context.Context, cfg config.Config, _ string) (*bootstrap.Runtime, error) {
		called = true
		return bootstrap.Mock(cfg)
	}}
	var out bytes.Buffer
	if err := execute(context.Background(), strings.NewReader(""), &out, []string{"doctor", "--helper"}, deps); err != nil {
		t.Fatal(err)
	}
	if !called || !strings.Contains(out.String(), "read-only PoC") {
		t.Fatalf("called=%t output=%q", called, out.String())
	}
}

func TestHelperLatencyPrintsTimingOnly(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	deps := dependencies{latency: func(_ context.Context, address string) (remote.LatencyResult, error) {
		if address != "192.0.2.10:43127" {
			t.Fatalf("address=%q", address)
		}
		return remote.LatencyResult{
			NotificationToMac: 187 * time.Millisecond,
			CalibrationRTT:    4 * time.Millisecond,
			ClockOffset:       82 * time.Millisecond,
		}, nil
	}}
	var out bytes.Buffer
	if err := execute(
		context.Background(),
		strings.NewReader(""),
		&out,
		[]string{"latency", "--helper", "--helper-address", "192.0.2.10:43127"},
		deps,
	); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, expected := range []string{
		"Latency probe armed",
		"notification_to_mac_ms=187",
		"calibration_rtt_ms=4",
		"clock_offset_ms=82",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("output=%q missing %q", text, expected)
		}
	}
}

func TestPairReadsCodePrivatelyAndDoesNotRequireHelperFlag(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	const code = "AAAQ-EAYE-AUDA-OCAJ-BIFQ-YDIO-B4IB-CEQT"
	var receivedAddress, receivedCode string
	deps := dependencies{pair: func(_ context.Context, address, pairingCode string) (remote.PairResult, error) {
		receivedAddress, receivedCode = address, pairingCode
		return remote.PairResult{DeviceName: "Galaxy Test"}, nil
	}}
	var out bytes.Buffer
	err := execute(context.Background(), strings.NewReader(code+"\n"), &out, []string{"pair", "--helper-address", "192.0.2.10:43127"}, deps)
	if err != nil {
		t.Fatal(err)
	}
	if receivedAddress != "192.0.2.10:43127" || receivedCode != code {
		t.Fatalf("address=%q code matched=%t", receivedAddress, receivedCode == code)
	}
	if strings.Contains(out.String(), code) || !strings.Contains(out.String(), "Paired with Galaxy Test") {
		t.Fatalf("output=%q", out.String())
	}
}

func TestPairJSONRemainsMachineReadableAndDoesNotPrintPrompt(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	deps := dependencies{pair: func(context.Context, string, string) (remote.PairResult, error) {
		return remote.PairResult{DeviceName: "Galaxy Test"}, nil
	}}
	var out bytes.Buffer
	err := execute(context.Background(), strings.NewReader("AAAQ-EAYE-AUDA-OCAJ-BIFQ-YDIO-B4IB-CEQT\n"), &out, []string{"pair", "--json"}, deps)
	if err != nil || !json.Valid(out.Bytes()) || strings.Contains(out.String(), "Pairing code") {
		t.Fatalf("output=%q err=%v", out.String(), err)
	}
}

func TestRealReadCommandsAndJSON(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	deps := dependencies{real: func(_ context.Context, cfg config.Config, _ string) (*bootstrap.Runtime, error) {
		return bootstrap.Mock(cfg)
	}}
	for _, args := range [][]string{{"conversations", "--json"}, {"unread", "--json"}, {"messages", "1", "--json"}} {
		var out bytes.Buffer
		if err := execute(context.Background(), strings.NewReader(""), &out, args, deps); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if !json.Valid(out.Bytes()) {
			t.Fatalf("%v: invalid JSON %q", args, out.String())
		}
	}
}

type fixedSender struct {
	result domain.SendResult
	err    error
}

func (s fixedSender) Send(context.Context, string, string) (domain.SendResult, error) {
	return s.result, s.err
}

func runtimeWithSender(sender domain.MessageSender) *bootstrap.Runtime {
	backend := mock.New()
	return &bootstrap.Runtime{Service: app.NewService(backend, sender, nil, nil, app.NotificationPolicy{}, domain.ApplicationStatus{Label: "USB"})}
}

func TestRealSendUsesRuntimeAndPrintsPrivacySafePlainSuccess(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	called := false
	deps := dependencies{real: func(context.Context, config.Config, string) (*bootstrap.Runtime, error) {
		called = true
		return runtimeWithSender(fixedSender{result: domain.SendResult{MessageID: 12345, ThreadID: 49}}), nil
	}}
	var out bytes.Buffer
	err := execute(context.Background(), strings.NewReader(""), &out, []string{"send", "--to", "01012345678", "--text", "private body"}, deps)
	if err != nil || !called || strings.TrimSpace(out.String()) != "Message sent." {
		t.Fatalf("out=%q err=%v called=%v", out.String(), err, called)
	}
	if strings.Contains(out.String(), "01012345678") || strings.Contains(out.String(), "private body") {
		t.Fatalf("send output leaked private values: %q", out.String())
	}
}

func TestRealSendJSONReturnsVerifiedIDs(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	deps := dependencies{real: func(context.Context, config.Config, string) (*bootstrap.Runtime, error) {
		return runtimeWithSender(fixedSender{result: domain.SendResult{MessageID: 12345, ThreadID: 49}}), nil
	}}
	var out bytes.Buffer
	if err := execute(context.Background(), strings.NewReader(""), &out, []string{"send", "--to", "01012345678", "--text", "synthetic", "--json"}, deps); err != nil {
		t.Fatal(err)
	}
	var response struct {
		Success   bool  `json:"success"`
		MessageID int64 `json:"message_id"`
		ThreadID  int64 `json:"thread_id"`
	}
	if err := json.Unmarshal(out.Bytes(), &response); err != nil || !response.Success || response.MessageID != 12345 || response.ThreadID != 49 {
		t.Fatalf("response=%+v out=%q err=%v", response, out.String(), err)
	}
}

func TestRealSendErrorsAreActionableAndLeaveJSONStdoutEmpty(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"scrcpy", domain.ErrScrcpyNotFound, "Install scrcpy"},
		{"clipboard", domain.ErrClipboardRead, "pbcopy and pbpaste"},
		{"verification", domain.ErrSendVerificationTimeout, "not verified"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			deps := dependencies{real: func(context.Context, config.Config, string) (*bootstrap.Runtime, error) {
				return runtimeWithSender(fixedSender{err: tc.err}), nil
			}}
			var out bytes.Buffer
			err := execute(context.Background(), strings.NewReader(""), &out, []string{"send", "--to", "01012345678", "--text", "synthetic", "--json"}, deps)
			if !errors.Is(err, tc.err) || !strings.Contains(err.Error(), tc.want) || out.Len() != 0 {
				t.Fatalf("out=%q err=%v", out.String(), err)
			}
		})
	}
}

func TestDoctorFormatsStructuredChecks(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	deps := dependencies{doctor: func(context.Context, config.Config, string) doctor.Report {
		return doctor.Report{
			Checks: []doctor.Check{
				{Name: "adb", Detail: "/synthetic/adb", State: doctor.Pass},
				{Name: "RCS", Detail: "deferred", State: doctor.Info},
			},
			Ready: true, ReadReady: true, SendReady: true, Summary: "Read: ready\nSend: ready",
		}
	}}
	var out bytes.Buffer
	if err := execute(context.Background(), strings.NewReader(""), &out, []string{"doctor"}, deps); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"GalaxyTTY Doctor", "✓ adb", "○ RCS", "Read: ready", "Send: ready"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %q in %q", want, out.String())
		}
	}
}

func TestDoctorDoesNotFailReadModeWhenOnlySendIsNotReady(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	deps := dependencies{doctor: func(context.Context, config.Config, string) doctor.Report {
		return doctor.Report{Ready: true, ReadReady: true, SendReady: false, Summary: "Read: ready\nSend: not ready"}
	}}
	var out bytes.Buffer
	if err := execute(context.Background(), strings.NewReader(""), &out, []string{"doctor"}, deps); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Send: not ready") {
		t.Fatalf("out=%q", out.String())
	}
}

func TestRealFactoryErrorLeavesJSONStdoutEmpty(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	deps := dependencies{real: func(context.Context, config.Config, string) (*bootstrap.Runtime, error) {
		return nil, errors.New("synthetic real failure")
	}}
	var out bytes.Buffer
	err := execute(context.Background(), strings.NewReader(""), &out, []string{"conversations", "--json"}, deps)
	if err == nil || out.Len() != 0 {
		t.Fatalf("out=%q err=%v", out.String(), err)
	}
}

func TestRealNoDeviceErrorIsActionable(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	deps := dependencies{real: func(context.Context, config.Config, string) (*bootstrap.Runtime, error) {
		return nil, adb.ErrNoDevices
	}}
	var out bytes.Buffer
	err := execute(context.Background(), strings.NewReader(""), &out, []string{"conversations", "--json"}, deps)
	if !errors.Is(err, adb.ErrNoDevices) {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(err.Error(), "Enable USB debugging") || !strings.Contains(err.Error(), "already-paired Wireless Debugging") {
		t.Fatalf("error is not actionable: %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("JSON stdout=%q", out.String())
	}
}
