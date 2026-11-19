package doctor

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/d56de/shrike/internal/core"
)

// Strikethrough styles a row rune by rune; any ANSI already inside the row (the
// coloured CPU bar, the Σ prefix, the severity label) gets split and shows up as
// literal "[38;2;…m" text.
func TestKilledRowHasNoBrokenEscapes(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	m := Model{
		Width:  120,
		Height: 40,
		Findings: []core.Finding{{
			Detector: "zombie",
			Severity: core.SeverityHigh,
			Process:  core.ProcessInfo{PID: 54058, Command: "caffeinate", CPUPercent: 40, ElapsedTime: 48 * time.Hour},
		}},
		Selected:   map[int]bool{},
		Paused:     map[int]core.ProcessInfo{},
		KilledPIDs: map[int]bool{54058: true},
	}
	for _, line := range strings.Split(ansi.Strip(m.View()), "\n") {
		if strings.Contains(line, "caffeinate") && strings.Contains(line, "[38;") {
			t.Fatalf("killed row leaks raw escape codes: %q", line)
		}
	}
}
