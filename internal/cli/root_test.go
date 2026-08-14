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

	"github.com/galaxytty/galaxytty/internal/bootstrap"
	"github.com/galaxytty/galaxytty/internal/config"
	"github.com/galaxytty/galaxytty/internal/doctor"
	"github.com/galaxytty/galaxytty/internal/domain"
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
func TestJSONOnly(t *testing.T) {
	for _, a := range [][]string{{"conversations", "--mock", "--json"}, {"unread", "--mock", "--json"}, {"messages", "1", "--mock", "--json"}} {
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

func TestRealSendRejectsBeforeRuntimeConstruction(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	called := false
	deps := dependencies{real: func(context.Context, config.Config, string) (*bootstrap.Runtime, error) {
		called = true
		return nil, errors.New("must not construct runtime")
	}}
	var out bytes.Buffer
	err := execute(context.Background(), strings.NewReader(""), &out, []string{"send", "--to", "synthetic", "--text", "synthetic"}, deps)
	if !errors.Is(err, domain.ErrSendingNotImplemented) || called {
		t.Fatalf("err=%v called=%v", err, called)
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
			Ready: true, Summary: "Read mode ready. Sending not implemented yet.",
		}
	}}
	var out bytes.Buffer
	if err := execute(context.Background(), strings.NewReader(""), &out, []string{"doctor"}, deps); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"GalaxyTTY Doctor", "✓ adb", "○ RCS", "Read mode ready"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %q in %q", want, out.String())
		}
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
