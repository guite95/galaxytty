package views

import "github.com/galaxytty/galaxytty/internal/tui/components"

func frame(top, body, bottom []string, width, height int) string {
	available := max(0, height-len(top)-len(bottom))
	if len(body) > available {
		body = body[:available]
	}
	lines := make([]string, 0, height)
	lines = append(lines, top...)
	lines = append(lines, body...)
	for len(lines) < height-len(bottom) {
		lines = append(lines, "")
	}
	lines = append(lines, bottom...)
	return components.FitLines(lines, width, height)
}
