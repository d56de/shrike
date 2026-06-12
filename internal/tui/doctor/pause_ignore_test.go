package doctor

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/d56de/shrike/internal/core"
)

// stubPause is an injectable Pause action that returns canned results without
// signalling any real process.
type stubPause struct {
	results []core.ActionResult
}

func (stubPause) Key() rune         { return 'p' }
func (stubPause) Name() string      { return "pause" }
func (stubPause) Confirm() string   { return "" }
func (stubPause) Destructive() bool { return false }
func (s stubPause) Execute(_ context.Context, _ []core.ProcessInfo) []core.ActionResult {
	return s.results
}

func keyRunes(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

func TestPause_OptimisticallyPinsThenResumes(t *testing.T) {
	pi := core.ProcessInfo{PID: 42, Command: "node", State: core.StateRunning, CPUPercent: 180}
	m := Model{
		Findings:    []core.Finding{{Detector: "runaway", Process: pi}},
		Selected:    map[int]bool{},
		Paused:      map[int]core.ProcessInfo{},
		KilledPIDs:  map[int]bool{},
		PauseAction: stubPause{results: []core.ActionResult{{PID: 42, Message: "paused"}}},
	}

	mm, cmd := m.Update(keyRunes('p'))
	m = mm.(Model)
	if !m.isPaused(42) {
		t.Fatal("expected PID 42 optimistically pinned after [p]")
	}
	if cmd == nil {
		t.Fatal("expected a pause command")
	}
	m2, _ := m.Update(cmd())
	m = m2.(Model)
	if !m.isPaused(42) {
		t.Error("expected PID 42 to remain paused after ActionMsg")
	}

	// The action left the results modal open; dismiss it back to the list
	// before resuming (pause/resume is a list-mode action).
	md, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = md.(Model)

	m.PauseAction = stubPause{results: []core.ActionResult{{PID: 42, Message: "resumed"}}}
	mm, cmd = m.Update(keyRunes('p'))
	m = mm.(Model)
	if cmd == nil {
		t.Fatal("expected a resume command")
	}
	m3, _ := m.Update(cmd())
	m = m3.(Model)
	if m.isPaused(42) {
		t.Error("expected PID 42 removed from Paused after resume")
	}
}

func TestPause_IgnoredInModalModes(t *testing.T) {
	m := Model{
		Mode:        ModeConfirm,
		Findings:    []core.Finding{{Detector: "runaway", Process: core.ProcessInfo{PID: 42, State: core.StateRunning}}},
		Selected:    map[int]bool{},
		Paused:      map[int]core.ProcessInfo{},
		KilledPIDs:  map[int]bool{},
		PauseAction: stubPause{results: []core.ActionResult{{PID: 42, Message: "paused"}}},
	}
	mm, _ := m.Update(keyRunes('p'))
	m = mm.(Model)
	if m.isPaused(42) {
		t.Error("expected [p] to be ignored while a confirm modal is open")
	}
}

func TestPause_PinSurvivesRescan(t *testing.T) {
	m := Model{
		Selected:   map[int]bool{},
		KilledPIDs: map[int]bool{},
		Paused:     map[int]core.ProcessInfo{42: {PID: 42, Command: "node"}},
	}
	m2, _ := m.Update(RescanDoneMsg{Findings: []core.Finding{}, Err: nil})
	m = m2.(Model)

	found := false
	for _, f := range m.Findings {
		if f.Detector == "paused" && f.Process.PID == 42 {
			found = true
		}
	}
	if !found {
		t.Error("expected synthetic paused finding for PID 42 after rescan")
	}
}

func TestActionMsg_KillStillMarksKilled(t *testing.T) {
	m := Model{
		Selected:   map[int]bool{},
		KilledPIDs: map[int]bool{},
		Paused:     map[int]core.ProcessInfo{},
	}
	m2, _ := m.Update(ActionMsg{Results: []core.ActionResult{{PID: 99, Message: "sent SIGTERM"}}})
	m = m2.(Model)
	if !m.KilledPIDs[99] {
		t.Error("expected PID 99 marked killed (kill path regression)")
	}
}
