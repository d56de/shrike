package doctor

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/d56de/shrike/internal/core"
)

func listModel(width int) Model {
	return Model{
		Width:  width,
		Height: 40,
		Findings: []core.Finding{{
			Detector: "runaway",
			Severity: core.SeverityHigh,
			Process:  core.ProcessInfo{PID: 4521, Command: "node-some-long-command", CPUPercent: 53, RSS: 1200 * 1024 * 1024},
		}},
		Selected:   map[int]bool{},
		Paused:     map[int]core.ProcessInfo{},
		KilledPIDs: map[int]bool{},
	}
}

// TestRow_NarrowDoesNotOverflow guards the doctor list rows against spilling
// past the frame on an 80-column terminal — the rows are otherwise a fixed
// ~94 columns wide and the right-hand fields get clipped/wrapped.
func TestRow_NarrowDoesNotOverflow(t *testing.T) {
	const width = 80
	out := listModel(width).View()
	for _, line := range strings.Split(out, "\n") {
		if w := lipgloss.Width(line); w > width {
			t.Errorf("list line exceeds terminal width %d (got %d): %q", width, w, line)
		}
	}
}

// TestRow_SeverityLabelResponsive verifies the redundant severity label (the
// bar is already severity-tinted) is dropped on narrow terminals and shown on
// wide ones.
func TestRow_SeverityLabelResponsive(t *testing.T) {
	if out := listModel(70).View(); strings.Contains(out, "High") {
		t.Errorf("expected severity label dropped on a narrow (70-col) terminal:\n%s", out)
	}
	if out := listModel(140).View(); !strings.Contains(out, "High") {
		t.Error("expected severity label shown on a wide (140-col) terminal")
	}
}
