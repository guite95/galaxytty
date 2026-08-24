package components

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestCellWidthUsesTerminalDisplayCells(t *testing.T) {
	tests := map[string]int{
		"A":   1,
		"가":   2,
		"😀":   2,
		"A가😀": 5,
	}
	for input, want := range tests {
		if got := CellWidth(input); got != want {
			t.Fatalf("CellWidth(%q)=%d want=%d", input, got, want)
		}
	}
}

func TestTruncateAndWrapRespectCellWidth(t *testing.T) {
	if got := Truncate("가나다라", 5); got != "가나…" {
		t.Fatalf("truncate=%q width=%d", got, CellWidth(got))
	}

	wrapped := Wrap("한글과 emoji 😀 그리고 a-very-long-word", 10)
	for _, line := range strings.Split(wrapped, "\n") {
		if got := CellWidth(line); got > 10 {
			t.Fatalf("line %q width=%d", line, got)
		}
	}
}

func TestAlignmentIgnoresANSISequences(t *testing.T) {
	styled := lipgloss.NewStyle().Foreground(lipgloss.Color("75")).Render("가😀")
	got := AlignRight(styled, 10)
	if width := CellWidth(got); width != 10 {
		t.Fatalf("aligned width=%d value=%q", width, got)
	}
	if !strings.HasPrefix(got, strings.Repeat(" ", 6)) {
		t.Fatalf("missing cell-aware padding: %q", got)
	}
}
