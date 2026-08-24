package components

import "github.com/charmbracelet/lipgloss"

type Theme struct {
	Brand     lipgloss.Style
	Heading   lipgloss.Style
	Text      lipgloss.Style
	Muted     lipgloss.Style
	Accent    lipgloss.Style
	Connected lipgloss.Style
	Warning   lipgloss.Style
	Error     lipgloss.Style
	Outgoing  lipgloss.Style
}

// DefaultTheme deliberately leaves backgrounds unset so the terminal's own
// background remains authoritative.
func DefaultTheme() Theme {
	return Theme{
		Brand:     lipgloss.NewStyle().Bold(true),
		Heading:   lipgloss.NewStyle().Bold(true),
		Text:      lipgloss.NewStyle(),
		Muted:     lipgloss.NewStyle().Foreground(lipgloss.Color("244")),
		Accent:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("75")),
		Connected: lipgloss.NewStyle().Foreground(lipgloss.Color("42")),
		Warning:   lipgloss.NewStyle().Foreground(lipgloss.Color("214")),
		Error:     lipgloss.NewStyle().Foreground(lipgloss.Color("203")),
		Outgoing:  lipgloss.NewStyle().Foreground(lipgloss.Color("81")),
	}
}
