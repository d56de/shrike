package doctor

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/d56de/shrike/internal/core"
)

func TestLoadSection(t *testing.T) {
	m := listModel(100)
	m.Load = core.Load{CPUPercent: 400, GPUPercent: 12, Procs: 3}
	m.Cores = 8
	out := m.View()
	if !strings.Contains(out, "CPU") || !strings.Contains(out, "50%") || !strings.Contains(out, "8 cores") {
		t.Errorf("expected CPU load 50%% of 8 cores:\n%s", out)
	}
	if !strings.Contains(out, "GPU") || !strings.Contains(out, "12%") {
		t.Errorf("expected GPU load 12%%:\n%s", out)
	}
	for _, line := range strings.Split(listModelNarrow(m).View(), "\n") {
		if w := lipgloss.Width(line); w > 60 {
			t.Errorf("load line overflows 60 cols (%d): %q", w, line)
		}
	}
}

func TestLoadSectionHiddenWithoutSnapshot(t *testing.T) {
	if out := listModel(100).View(); strings.Contains(out, "cores") {
		t.Errorf("load section must not render before a snapshot:\n%s", out)
	}
}

func listModelNarrow(m Model) Model { m.Width = 60; return m }
