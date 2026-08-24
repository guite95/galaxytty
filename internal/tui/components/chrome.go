package components

import (
	"strings"
)

func Header(theme Theme, width int, device, state string) string {
	device = strings.TrimSpace(device)
	if device == "" {
		device = "Galaxy"
	}
	dot := theme.Connected.Render("●")
	if !isConnected(state) {
		dot = theme.Warning.Render("●")
	}
	return Columns(theme.Brand.Render("GalaxyTTY"), dot+" "+theme.Muted.Render(device), width)
}

func ConnectionStatus(theme Theme, state string) string {
	label := connectionLabel(state)
	dot := theme.Connected.Render("●")
	if label != "connected" {
		dot = theme.Warning.Render("●")
	}
	return dot + " " + theme.Muted.Render(label)
}

func ErrorLine(theme Theme, value string, width int) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return theme.Error.Render(Truncate(value, width))
}

func isConnected(state string) bool {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "", "connected", "ready", "usb", "wireless", "mock connected":
		return true
	default:
		return false
	}
}

func connectionLabel(state string) string {
	state = strings.ToLower(strings.TrimSpace(state))
	switch state {
	case "", "connected", "ready", "usb", "wireless", "mock connected":
		return "connected"
	case "connecting", "reconnecting":
		return state
	case "unauthorized":
		return "authorization required"
	case "sending…", "sending...":
		return "sending"
	default:
		return "offline"
	}
}
