package config

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Duration struct{ time.Duration }

func (d *Duration) UnmarshalText(v []byte) error {
	x, e := time.ParseDuration(string(v))
	d.Duration = x
	return e
}

type Config struct {
	Connection struct {
		PreferUSB bool `toml:"prefer_usb"`
	} `toml:"connection"`
	Polling struct {
		Interval Duration `toml:"interval"`
	} `toml:"polling"`
	Notifications struct {
		Enabled         bool `toml:"enabled"`
		ShowWhenFocused bool `toml:"show_when_focused"`
	} `toml:"notifications"`
	Samsung struct {
		DisplayWidth  int `toml:"display_width"`
		DisplayHeight int `toml:"display_height"`
		Layout        struct {
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
	f, e := os.Open(path)
	if errors.Is(e, os.ErrNotExist) {
		return c, nil
	}
	if e != nil {
		return c, e
	}
	defer f.Close()
	section := ""
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(strings.SplitN(s.Text(), "#", 2)[0])
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") {
			section = strings.Trim(line, "[] ")
			continue
		}
		p := strings.SplitN(line, "=", 2)
		if len(p) != 2 {
			continue
		}
		key, val := strings.TrimSpace(p[0]), strings.Trim(strings.TrimSpace(p[1]), "\"")
		switch section + "." + key {
		case "connection.prefer_usb":
			c.Connection.PreferUSB, e = strconv.ParseBool(val)
		case "polling.interval":
			e = c.Polling.Interval.UnmarshalText([]byte(val))
		case "notifications.enabled":
			c.Notifications.Enabled, e = strconv.ParseBool(val)
		case "notifications.show_when_focused":
			c.Notifications.ShowWhenFocused, e = strconv.ParseBool(val)
		case "samsung.display_width":
			c.Samsung.DisplayWidth, _ = strconv.Atoi(val)
		case "samsung.display_height":
			c.Samsung.DisplayHeight, _ = strconv.Atoi(val)
		case "samsung.layout.composer_x":
			c.Samsung.Layout.ComposerX, _ = strconv.Atoi(val)
		case "samsung.layout.composer_y":
			c.Samsung.Layout.ComposerY, _ = strconv.Atoi(val)
		case "samsung.layout.send_x":
			c.Samsung.Layout.SendX, _ = strconv.Atoi(val)
		case "samsung.layout.send_y":
			c.Samsung.Layout.SendY, _ = strconv.Atoi(val)
		}
		if e != nil {
			return c, e
		}
	}
	return c, s.Err()
}
