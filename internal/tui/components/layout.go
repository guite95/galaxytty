package components

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// CellWidth reports terminal display cells, ignoring ANSI sequences and
// treating grapheme clusters such as Korean and emoji as wide characters.
func CellWidth(value string) int { return ansi.StringWidth(value) }

func Truncate(value string, width int) string {
	if width <= 0 {
		return ""
	}
	return ansi.Truncate(value, width, "…")
}

func Clip(value string, width int) string {
	if width <= 0 {
		return ""
	}
	return ansi.Truncate(value, width, "")
}

func Wrap(value string, width int) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	if width <= 0 {
		return ""
	}
	return ansi.Wrap(value, width, "")
}

func AlignRight(value string, width int) string {
	value = Clip(value, width)
	return strings.Repeat(" ", max(0, width-CellWidth(value))) + value
}

func PadRight(value string, width int) string {
	value = Clip(value, width)
	return value + strings.Repeat(" ", max(0, width-CellWidth(value)))
}

// Columns pins right to the final cell while allowing the left content to use
// the remaining space. Both inputs may contain ANSI styling.
func Columns(left, right string, width int) string {
	if width <= 0 {
		return ""
	}
	right = Clip(right, width)
	rightWidth := CellWidth(right)
	if rightWidth == 0 {
		return PadRight(left, width)
	}
	leftWidth := max(0, width-rightWidth-1)
	left = Clip(left, leftWidth)
	gap := max(1, width-CellWidth(left)-rightWidth)
	return left + strings.Repeat(" ", gap) + right
}

func FitLines(lines []string, width, height int) string {
	if height <= 0 {
		return ""
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	for index := range lines {
		lines[index] = PadRight(lines[index], width)
	}
	return strings.Join(lines, "\n")
}
