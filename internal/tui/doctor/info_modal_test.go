package doctor

import (
	"testing"

	"github.com/d56de/shrike/internal/actions"
	"github.com/d56de/shrike/internal/core"
	"github.com/d56de/shrike/internal/tui/style"
)

// TestInfoDoneMsg_DiscardsStaleResponses verifies the PID guard: when the
// user closes info on PID A, navigates, and opens info on PID B before A's
// fetch returns, the stale message must not overwrite B's pending state.
func TestInfoDoneMsg_DiscardsStaleResponses(t *testing.T) {
	m := Model{
		InfoLoading:   true,
		InfoTargetPID: 200, // user is now waiting for PID 200
		Theme:         style.DefaultTheme(),
	}
	stale := InfoDoneMsg{
		PID:     100, // late response from a previous fetch
		Details: actions.InfoDetails{PID: 100, Threads: 99, Cwd: "/old"},
	}
	updated, _ := m.Update(stale)
	got := updated.(Model)
	if !got.InfoLoading {
		t.Error("loading state should stay set; stale message must be ignored")
	}
	if got.InfoDetails.Cwd == "/old" {
		t.Error("stale details leaked into model state")
	}
}

// TestInfoDoneMsg_AcceptsMatchingPID covers the happy path.
func TestInfoDoneMsg_AcceptsMatchingPID(t *testing.T) {
	m := Model{
		InfoLoading:   true,
		InfoTargetPID: 100,
		Theme:         style.DefaultTheme(),
	}
	msg := InfoDoneMsg{
		PID:     100,
		Details: actions.InfoDetails{PID: 100, Threads: 7, Cwd: "/tmp"},
	}
	updated, _ := m.Update(msg)
	got := updated.(Model)
	if got.InfoLoading {
		t.Error("loading flag should clear when matching response arrives")
	}
	if got.InfoDetails.Threads != 7 || got.InfoDetails.Cwd != "/tmp" {
		t.Errorf("details not applied: %+v", got.InfoDetails)
	}
}

// TestRenderInfoBody_ShowsLoadingThenDetails covers both the spinner-during-
// loading state and the populated state — the modal must NEVER blank out.
func TestRenderInfoBody_ShowsLoadingThenDetails(t *testing.T) {
	loading := Model{
		Findings:    []core.Finding{{Process: core.ProcessInfo{PID: 42, Command: "stub"}}},
		Cursor:      0,
		InfoLoading: true,
		Theme:       style.DefaultTheme(),
	}
	out := renderInfoBody(loading.Theme, loading, 80)
	if !containsAll(out, "details", "loading process details") {
		t.Errorf("loading state should show spinner + label, got:\n%s", out)
	}

	done := Model{
		Findings: []core.Finding{{Process: core.ProcessInfo{PID: 42, Command: "stub"}}},
		Cursor:   0,
		InfoDetails: actions.InfoDetails{
			PID: 42, Cwd: "/home/x", Threads: 12, IOReads: 1024, IOWrites: 2048,
			Ancestors: []actions.Ancestor{{PID: 1, Command: "launchd"}},
		},
		Theme: style.DefaultTheme(),
	}
	out = renderInfoBody(done.Theme, done, 80)
	for _, want := range []string{
		"Cwd:", "/home/x", "Threads:", "12",
		"I/O read:", "1.0 KB", "I/O write:", "2.0 KB",
		"Ancestry:", "launchd(1)",
	} {
		if !containsAll(out, want) {
			t.Errorf("rendered modal missing %q:\n%s", want, out)
		}
	}
}

func containsAll(s string, needles ...string) bool {
	for _, n := range needles {
		if !contains(s, n) {
			return false
		}
	}
	return true
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
