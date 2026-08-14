package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/galaxytty/galaxytty/internal/domain"
)

func write(t *testing.T, s string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.toml")
	if e := os.WriteFile(p, []byte(s), 0600); e != nil {
		t.Fatal(e)
	}
	return p
}
func TestDefaultsAndMissing(t *testing.T) {
	c, e := Load(filepath.Join(t.TempDir(), "missing"))
	if e != nil || !c.Connection.PreferUSB || c.Polling.Interval.Duration != time.Second || !c.Notifications.Enabled || c.Notifications.ShowWhenFocused || c.Samsung.Layout.SendX != 1004 || c.Samsung.Layout.SendY != 955 {
		t.Fatal(c, e)
	}
	if c.Samsung.ClipboardSyncDelay.Duration != 300*time.Millisecond || c.Samsung.SendSettleDelay.Duration != 500*time.Millisecond || c.Samsung.VerificationTimeout.Duration != 10*time.Second {
		t.Fatalf("Samsung timings=%+v", c.Samsung)
	}
	if c.Samsung.TextInputMode != domain.TextInputIntentBody {
		t.Fatalf("text input mode=%q", c.Samsung.TextInputMode)
	}
}
func TestValidTOML(t *testing.T) {
	p := write(t, "[connection]\nprefer_usb=false\n[polling]\ninterval='2s'\n[notifications]\nenabled=false\nshow_when_focused=true\n[samsung]\ntext_input_mode='clipboard'\ndisplay_width=720\ndisplay_height=1280\nclipboard_sync_delay='450ms'\nsend_settle_delay='250ms'\nverification_timeout='12s'\n[samsung.layout]\ncomposer_x=1\ncomposer_y=2\nsend_x=3\nsend_y=4\n")
	c, e := Load(p)
	if e != nil || c.Connection.PreferUSB || c.Polling.Interval.Duration != 2*time.Second || c.Samsung.Layout.SendY != 4 || c.Samsung.ClipboardSyncDelay.Duration != 450*time.Millisecond || c.Samsung.VerificationTimeout.Duration != 12*time.Second || c.Samsung.TextInputMode != domain.TextInputClipboard {
		t.Fatal(c, e)
	}
}
func TestInvalidConfig(t *testing.T) {
	for _, s := range []string{"[broken", "[polling]\ninterval='never'", "[connection]\nprefer_usb='yes'", "[samsung]\ndisplay_width='wide'", "[samsung]\ntext_input_mode='magic'", "unknown=1"} {
		if _, e := Load(write(t, s)); e == nil {
			t.Fatalf("expected error for %q", s)
		}
	}
}

func TestValidateRejectsInvalidSamsungDisplayLayoutAndTimings(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*Config)
	}{
		{"width", func(c *Config) { c.Samsung.DisplayWidth = 0 }},
		{"height", func(c *Config) { c.Samsung.DisplayHeight = -1 }},
		{"composer x", func(c *Config) { c.Samsung.Layout.ComposerX = c.Samsung.DisplayWidth }},
		{"composer y", func(c *Config) { c.Samsung.Layout.ComposerY = -1 }},
		{"send x", func(c *Config) { c.Samsung.Layout.SendX = -1 }},
		{"send y", func(c *Config) { c.Samsung.Layout.SendY = c.Samsung.DisplayHeight }},
		{"clipboard delay", func(c *Config) { c.Samsung.ClipboardSyncDelay.Duration = 0 }},
		{"settle delay", func(c *Config) { c.Samsung.SendSettleDelay.Duration = -time.Millisecond }},
		{"verification timeout", func(c *Config) { c.Samsung.VerificationTimeout.Duration = 0 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Default()
			tc.mutate(&cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}
func TestPath(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/custom")
	p, e := Path()
	if e != nil || p != "/tmp/custom/galaxytty/config.toml" {
		t.Fatal(p, e)
	}
}

func TestDeviceSelector(t *testing.T) {
	c, err := Load(write(t, "[connection]\nprefer_usb=false\ndevice='synthetic-target'\n"))
	if err != nil || c.Connection.Device != "synthetic-target" {
		t.Fatalf("device=%q err=%v", c.Connection.Device, err)
	}
}
