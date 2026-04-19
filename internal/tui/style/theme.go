// Package style centralises Lipgloss styles for shrike's TUI.
package style

import "github.com/charmbracelet/lipgloss"

// Theme collects the Lipgloss styles used across the doctor screen.
type Theme struct {
	Border         lipgloss.Style
	Frame          lipgloss.Style // border characters (╭──╮│╰──╯)
	Title          lipgloss.Style
	Subtle         lipgloss.Style
	Cursor         lipgloss.Style // active row cursor
	CursorInactive lipgloss.Style // non-active row cursor glyph
	Severity       map[string]lipgloss.Style
	CheckboxOn     lipgloss.Style // selected
	CheckboxOff    lipgloss.Style // unselected
	KeyHint        lipgloss.Style
	Gutter         lipgloss.Style // left tree-gutter "│ "
	CPUBarFilled   lipgloss.Style
	CPUBarEmpty    lipgloss.Style
	Accent         lipgloss.Style // status dot, active highlights
}

// DefaultTheme returns the baseline Lipgloss theme.
func DefaultTheme() Theme {
	return Theme{
		// Border is unused now — we build the frame manually to embed a title.
		Border:         lipgloss.NewStyle(),
		Frame:          lipgloss.NewStyle().Foreground(lipgloss.Color("#8A88C2")),
		Title:          lipgloss.NewStyle().Foreground(lipgloss.Color("#8A88C2")).Bold(true),
		Subtle:         lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
		Cursor:         lipgloss.NewStyle().Foreground(lipgloss.Color("#49A281")).Bold(true),
		CursorInactive: lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		CheckboxOn:     lipgloss.NewStyle().Foreground(lipgloss.Color("#7EE787")).Bold(true),
		CheckboxOff:    lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		KeyHint:        lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
		Gutter:         lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		CPUBarFilled:   lipgloss.NewStyle().Foreground(lipgloss.Color("#FD8282")),
		CPUBarEmpty:    lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		Accent:         lipgloss.NewStyle().Foreground(lipgloss.Color("#8A88C2")).Bold(true),
		Severity: map[string]lipgloss.Style{
			"critical": lipgloss.NewStyle().Foreground(lipgloss.Color("#FD8282")).Bold(true),
			"high":     lipgloss.NewStyle().Foreground(lipgloss.Color("#FD8282")),
			"medium":   lipgloss.NewStyle().Foreground(lipgloss.Color("#FF9F72")),
			"low":      lipgloss.NewStyle().Foreground(lipgloss.Color("#FEFE7E")),
		},
	}
}

// FromConfig returns a theme with severity colours overridden by the given
// hex strings. Empty strings fall back to DefaultTheme colours.
func FromConfig(high, medium, low string) Theme {
	t := DefaultTheme()
	if high != "" {
		t.Severity["high"] = lipgloss.NewStyle().Foreground(lipgloss.Color(high))
		t.Severity["critical"] = lipgloss.NewStyle().Foreground(lipgloss.Color(high)).Bold(true)
	}
	if medium != "" {
		t.Severity["medium"] = lipgloss.NewStyle().Foreground(lipgloss.Color(medium))
	}
	if low != "" {
		t.Severity["low"] = lipgloss.NewStyle().Foreground(lipgloss.Color(low))
	}
	return t
}
