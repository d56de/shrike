package doctor

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/d56de/shrike/internal/core"
	"github.com/d56de/shrike/internal/tui/style"
)

// TestInit_StartsTickerWhenAutoRefreshOn verifies that opening the doctor
// TUI with auto-refresh configured kicks off the tea.Tick chain immediately.
func TestInit_StartsTickerWhenAutoRefreshOn(t *testing.T) {
	on := Model{AutoRefreshInterval: 5 * time.Second, AutoRefreshOn: true}
	if on.Init() == nil {
		t.Error("Init should return a tick cmd when auto-refresh is enabled")
	}

	off := Model{AutoRefreshInterval: 5 * time.Second, AutoRefreshOn: false}
	if off.Init() != nil {
		t.Error("Init should be a no-op when auto-refresh is disabled")
	}

	zero := Model{AutoRefreshInterval: 0, AutoRefreshOn: true}
	if zero.Init() != nil {
		t.Error("Init should be a no-op when interval is zero")
	}
}

// TestAutoRefreshTick_SkipsWhenBusyButKeepsTicking guards the busy-skip path:
// modal open / action running / sample / rescan all defer the rescan but
// must NOT break the tick chain.
func TestAutoRefreshTick_SkipsWhenBusyButKeepsTicking(t *testing.T) {
	busyModes := []struct {
		name string
		m    Model
	}{
		{"confirm modal", Model{Mode: ModeConfirm, AutoRefreshOn: true, AutoRefreshInterval: 5 * time.Second}},
		{"already rescanning", Model{Mode: ModeList, Rescanning: true, AutoRefreshOn: true, AutoRefreshInterval: 5 * time.Second}},
		{"action running", Model{Mode: ModeList, ActionRunning: true, AutoRefreshOn: true, AutoRefreshInterval: 5 * time.Second}},
		{"sampling", Model{Mode: ModeList, Sampling: true, AutoRefreshOn: true, AutoRefreshInterval: 5 * time.Second}},
	}
	for _, tc := range busyModes {
		t.Run(tc.name, func(t *testing.T) {
			updated, cmd := tc.m.Update(autoRefreshTickMsg{})
			got := updated.(Model)
			if got.Rescanning && !tc.m.Rescanning {
				t.Errorf("busy state %q should NOT start a new rescan", tc.name)
			}
			if cmd == nil {
				t.Errorf("busy state %q should still schedule the next tick", tc.name)
			}
		})
	}
}

// TestAutoRefreshTick_DroppedWhenToggledOff covers the case where the user
// pressed [a] while a tick was already in flight: the message arrives but
// the toggle has already flipped, so nothing should happen.
func TestAutoRefreshTick_DroppedWhenToggledOff(t *testing.T) {
	m := Model{Mode: ModeList, AutoRefreshOn: false, AutoRefreshInterval: 5 * time.Second}
	updated, cmd := m.Update(autoRefreshTickMsg{})
	got := updated.(Model)
	if got.Rescanning {
		t.Error("tick arriving after toggle-off must not start a rescan")
	}
	if cmd != nil {
		t.Error("tick arriving after toggle-off must not re-arm the ticker")
	}
}

// TestToggleAutoRefresh_NoOpWhenNoInterval guards the keystroke being a
// bounce when no interval is configured — toggling visibility of an
// inert state would be confusing.
func TestToggleAutoRefresh_NoOpWhenNoInterval(t *testing.T) {
	m := Model{Mode: ModeList, AutoRefreshOn: false, AutoRefreshInterval: 0, Theme: style.DefaultTheme()}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	got := updated.(Model)
	if got.AutoRefreshOn {
		t.Error("[a] should not enable auto-refresh when interval is zero")
	}
}

// TestRescanDoneMsg_PreservesCursorByPID is the auto-refresh-friendly
// behaviour: when a rescan returns the same PID at a different index, the
// cursor follows it rather than jumping to row 0.
func TestRescanDoneMsg_PreservesCursorByPID(t *testing.T) {
	m := Model{
		Findings: []core.Finding{
			{Process: core.ProcessInfo{PID: 100}},
			{Process: core.ProcessInfo{PID: 200}}, // cursor here
			{Process: core.ProcessInfo{PID: 300}},
		},
		Cursor: 1,
		Theme:  style.DefaultTheme(),
	}
	// PID 200 moved up to index 0; PID 100 dropped off; new PID 400 appeared.
	msg := RescanDoneMsg{
		Findings: []core.Finding{
			{Process: core.ProcessInfo{PID: 200}},
			{Process: core.ProcessInfo{PID: 300}},
			{Process: core.ProcessInfo{PID: 400}},
		},
		Duration: 50 * time.Millisecond,
	}
	updated, _ := m.Update(msg)
	got := updated.(Model)
	if got.Cursor != 0 {
		t.Errorf("cursor should follow PID 200 to its new index 0, got %d", got.Cursor)
	}
}

// TestRescanDoneMsg_FallsBackToLastIndexWhenPIDGone covers the case where
// the watched process has exited — pick the last finding rather than 0 so
// the user lands at a sensible place if they were scrolled down.
func TestRescanDoneMsg_FallsBackToLastIndexWhenPIDGone(t *testing.T) {
	m := Model{
		Findings: []core.Finding{
			{Process: core.ProcessInfo{PID: 100}},
			{Process: core.ProcessInfo{PID: 200}},
		},
		Cursor: 1,
		Theme:  style.DefaultTheme(),
	}
	msg := RescanDoneMsg{
		Findings: []core.Finding{
			{Process: core.ProcessInfo{PID: 300}},
		},
		Duration: 50 * time.Millisecond,
	}
	updated, _ := m.Update(msg)
	got := updated.(Model)
	if got.Cursor != 0 || len(got.Findings) != 1 {
		t.Errorf("expected cursor=0 in single-finding list, got cursor=%d len=%d",
			got.Cursor, len(got.Findings))
	}
}
