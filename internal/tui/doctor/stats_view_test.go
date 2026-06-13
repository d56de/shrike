package doctor

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/d56de/shrike/internal/core"
)

func TestStats_KeyOpensAndAnyKeyCloses(t *testing.T) {
	m := Model{
		Findings:   []core.Finding{{Detector: "runaway", Process: core.ProcessInfo{PID: 1, Command: "node"}}},
		Selected:   map[int]bool{},
		Paused:     map[int]core.ProcessInfo{},
		KilledPIDs: map[int]bool{},
	}

	// [t] opens the trends view.
	mm, _ := m.Update(keyRunes('t'))
	m = mm.(Model)
	if m.Mode != ModeStats {
		t.Fatalf("expected ModeStats after [t], got %v", m.Mode)
	}

	// Any key returns to the list.
	mm, _ = m.Update(keyRunes('x'))
	m = mm.(Model)
	if m.Mode != ModeList {
		t.Fatalf("expected ModeList after a key in ModeStats, got %v", m.Mode)
	}

	// esc also returns to the list.
	m.Mode = ModeStats
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = mm.(Model)
	if m.Mode != ModeList {
		t.Fatalf("expected ModeList after esc in ModeStats, got %v", m.Mode)
	}
}
