package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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
	if e != nil || !c.Connection.PreferUSB || c.Polling.Interval.Duration != time.Second || !c.Notifications.Enabled || c.Notifications.ShowWhenFocused || c.Samsung.Layout.SendX != 1004 {
		t.Fatal(c, e)
	}
}
func TestValidTOML(t *testing.T) {
	p := write(t, "[connection]\nprefer_usb=false\n[polling]\ninterval='2s'\n[notifications]\nenabled=false\nshow_when_focused=true\n[samsung]\ndisplay_width=720\ndisplay_height=1280\n[samsung.layout]\ncomposer_x=1\ncomposer_y=2\nsend_x=3\nsend_y=4\n")
	c, e := Load(p)
	if e != nil || c.Connection.PreferUSB || c.Polling.Interval.Duration != 2*time.Second || c.Samsung.Layout.SendY != 4 {
		t.Fatal(c, e)
	}
}
func TestInvalidConfig(t *testing.T) {
	for _, s := range []string{"[broken", "[polling]\ninterval='never'", "[connection]\nprefer_usb='yes'", "[samsung]\ndisplay_width='wide'", "unknown=1"} {
		if _, e := Load(write(t, s)); e == nil {
			t.Fatalf("expected error for %q", s)
		}
	}
}
func TestPath(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/custom")
	p, e := Path()
	if e != nil || p != "/tmp/custom/galaxytty/config.toml" {
		t.Fatal(p, e)
	}
}
