// Package style centralises Lipgloss styles for shrike's TUI.
package style

import "github.com/charmbracelet/lipgloss"

// Theme collects the Lipgloss styles used across the doctor screen.
type Theme struct {
	Border      lipgloss.Style
	Title       lipgloss.Style
	Subtle      lipgloss.Style
	Cursor      lipgloss.Style
	Severity    map[string]lipgloss.Style
	CheckboxOn  lipgloss.Style
	CheckboxOff lipgloss.Style
	KeyHint     lipgloss.Style
}

// DefaultTheme returns the baseline Lipgloss theme.
func DefaultTheme() Theme {
	return Theme{
		Border:      lipgloss.NewStyle().Border(lipgloss.DoubleBorder()).Padding(1, 2),
		Title:       lipgloss.NewStyle().Bold(true),
		Subtle:      lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
		Cursor:      lipgloss.NewStyle().Foreground(lipgloss.Color("#7EE787")).Bold(true),
		CheckboxOn:  lipgloss.NewStyle().Foreground(lipgloss.Color("#7EE787")),
		CheckboxOff: lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		KeyHint:     lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
		Severity: map[string]lipgloss.Style{
			"critical": lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5555")).Bold(true),
			"high":     lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5555")),
			"medium":   lipgloss.NewStyle().Foreground(lipgloss.Color("#ffa500")),
			"low":      lipgloss.NewStyle().Foreground(lipgloss.Color("#ffd700")),
		},
	}
}
