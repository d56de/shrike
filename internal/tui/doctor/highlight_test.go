package doctor

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/d56de/shrike/internal/core"
	"github.com/muesli/termenv"
)

const cursorGlyph = "\x1b[1;"

func TestActiveRowHighlighted(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	m := listModel(100)
	m.Findings = append(m.Findings, core.Finding{Detector: "runaway", Process: core.ProcessInfo{PID: 7, Command: "other"}})
	bg, _, _ := strings.Cut(lipgloss.NewStyle().Background(rowHighlight).Render(" "), " ")
	var active, other string
	for _, line := range strings.Split(m.View(), "\n") {
		switch {
		case strings.Contains(line, "node-some-long-command"):
			active = line
		case strings.Contains(line, "other"):
			other = line
		}
	}
	// Every reset inside the active row must re-open the background, else
	// the highlight breaks after the first coloured segment.
	if !strings.Contains(active, bg+cursorGlyph) || strings.Count(active, "\x1b[0m"+bg) < 3 {
		t.Errorf("active row not fully highlighted: %q", active)
	}
	if strings.Contains(other, bg) {
		t.Errorf("inactive row must not be highlighted: %q", other)
	}
}
