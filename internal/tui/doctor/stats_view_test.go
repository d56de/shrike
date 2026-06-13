package doctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

func TestStats_RenderEmptyHistory(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir()) // empty → no history
	m := Model{
		Width:      120,
		Height:     40,
		Mode:       ModeStats,
		Selected:   map[int]bool{},
		Paused:     map[int]core.ProcessInfo{},
		KilledPIDs: map[int]bool{},
	}
	out := m.View()
	if !strings.Contains(out, "no activity") {
		t.Errorf("expected 'no activity' in empty-history trends view, got:\n%s", out)
	}
	if !strings.Contains(out, "[esc] back") {
		t.Errorf("expected '[esc] back' footer in trends view")
	}
}

func TestStats_RenderSeededHistory(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)
	statedir := filepath.Join(dir, "shrike")
	if err := os.MkdirAll(statedir, 0o755); err != nil {
		t.Fatal(err)
	}
	ts := time.Now().UTC().Format(time.RFC3339)
	line := `{"_type":"run","ts":"` + ts + `","mode":"watch","procs_scanned":1,"duration_ms":2}` + "\n"
	if err := os.WriteFile(filepath.Join(statedir, "history.jsonl"), []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
	m := Model{
		Width:      120,
		Height:     40,
		Mode:       ModeStats,
		Selected:   map[int]bool{},
		Paused:     map[int]core.ProcessInfo{},
		KilledPIDs: map[int]bool{},
	}
	out := m.View()
	if !strings.Contains(out, "Less") || !strings.Contains(out, "More") {
		t.Errorf("expected heatmap legend (Less/More) for seeded history, got:\n%s", out)
	}
}

// TestStats_RenderNarrowDoesNotOverflow guards against the fixed-width
// legend+summary line spilling past the frame on an 80-column terminal (which
// would hard-wrap and corrupt the Bubble Tea diff renderer).
func TestStats_RenderNarrowDoesNotOverflow(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)
	statedir := filepath.Join(dir, "shrike")
	if err := os.MkdirAll(statedir, 0o755); err != nil {
		t.Fatal(err)
	}
	ts := time.Now().UTC().Format(time.RFC3339)
	var sb strings.Builder
	for i := 0; i < 3; i++ {
		sb.WriteString(`{"_type":"run","ts":"` + ts + `","mode":"watch"}` + "\n")
		sb.WriteString(`{"_type":"finding","ts":"` + ts + `","detector":"runaway","severity":"high","pid":1,"command":"node","cpu":99,"rss":1,"elapsed_s":1}` + "\n")
	}
	if err := os.WriteFile(filepath.Join(statedir, "history.jsonl"), []byte(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}

	const width = 80
	m := Model{
		Width:      width,
		Height:     40,
		Mode:       ModeStats,
		Selected:   map[int]bool{},
		Paused:     map[int]core.ProcessInfo{},
		KilledPIDs: map[int]bool{},
	}
	for _, line := range strings.Split(m.View(), "\n") {
		if w := lipgloss.Width(line); w > width {
			t.Errorf("trends line exceeds terminal width %d (got %d): %q", width, w, line)
		}
	}
}
