package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/pelletier/go-toml/v2"
)

type Duration struct{ time.Duration }

func (d *Duration) UnmarshalText(v []byte) error {
	x, e := time.ParseDuration(string(v))
	d.Duration = x
	return e
}

type Config struct {
	Connection struct {
		PreferUSB bool   `toml:"prefer_usb"`
		Device    string `toml:"device"`
	} `toml:"connection"`
	Polling struct {
		Interval Duration `toml:"interval"`
	} `toml:"polling"`
	Notifications struct {
		Enabled         bool `toml:"enabled"`
		ShowWhenFocused bool `toml:"show_when_focused"`
	} `toml:"notifications"`
	Samsung struct {
		DisplayWidth        int      `toml:"display_width"`
		DisplayHeight       int      `toml:"display_height"`
		ClipboardSyncDelay  Duration `toml:"clipboard_sync_delay"`
		SendSettleDelay     Duration `toml:"send_settle_delay"`
		VerificationTimeout Duration `toml:"verification_timeout"`
		Layout              struct {
			ComposerX int `toml:"composer_x"`
			ComposerY int `toml:"composer_y"`
			SendX     int `toml:"send_x"`
			SendY     int `toml:"send_y"`
		} `toml:"layout"`
	} `toml:"samsung"`
}

func Default() Config {
	var c Config
	c.Connection.PreferUSB = true
	c.Polling.Interval.Duration = time.Second
	c.Notifications.Enabled = true
	c.Samsung.DisplayWidth = 1080
	c.Samsung.DisplayHeight = 1920
	c.Samsung.ClipboardSyncDelay.Duration = 300 * time.Millisecond
	c.Samsung.SendSettleDelay.Duration = 200 * time.Millisecond
	c.Samsung.VerificationTimeout.Duration = 10 * time.Second
	c.Samsung.Layout.ComposerX = 500
	c.Samsung.Layout.ComposerY = 1800
	c.Samsung.Layout.SendX = 1004
	c.Samsung.Layout.SendY = 1273
	return c
}
func Path() (string, error) {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "galaxytty", "config.toml"), nil
	}
	h, e := os.UserHomeDir()
	if e != nil {
		return "", e
	}
	return filepath.Join(h, ".config", "galaxytty", "config.toml"), nil
}
func Load(path string) (Config, error) {
	c := Default()
	b, e := os.ReadFile(path)
	if errors.Is(e, os.ErrNotExist) {
		return c, c.Validate()
	}
	if e != nil {
		return c, e
	}
	dec := toml.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if e = dec.Decode(&c); e != nil {
		return c, fmt.Errorf("decode config: %w", e)
	}
	if e = c.Validate(); e != nil {
		return c, fmt.Errorf("validate config: %w", e)
	}
	return c, nil
}

func (c Config) Validate() error {
	if c.Samsung.DisplayWidth <= 0 || c.Samsung.DisplayHeight <= 0 {
		return fmt.Errorf("Samsung display dimensions must be positive")
	}
	for _, item := range []struct {
		name string
		x, y int
	}{
		{"composer", c.Samsung.Layout.ComposerX, c.Samsung.Layout.ComposerY},
		{"send", c.Samsung.Layout.SendX, c.Samsung.Layout.SendY},
	} {
		if item.x < 0 || item.x >= c.Samsung.DisplayWidth || item.y < 0 || item.y >= c.Samsung.DisplayHeight {
			return fmt.Errorf("Samsung %s coordinate is outside the display", item.name)
		}
	}
	if c.Samsung.ClipboardSyncDelay.Duration <= 0 || c.Samsung.SendSettleDelay.Duration <= 0 || c.Samsung.VerificationTimeout.Duration <= 0 {
		return fmt.Errorf("Samsung send timings must be positive")
	}
	return nil
}
