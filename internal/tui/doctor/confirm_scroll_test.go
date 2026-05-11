package doctor

import (
	"context"
	"strings"
	"testing"

	"github.com/d56de/shrike/internal/core"
	"github.com/d56de/shrike/internal/tui/style"
)

type stubKill struct{}

func (stubKill) Key() rune                                                       { return 'k' }
func (stubKill) Name() string                                                    { return "kill" }
func (stubKill) Confirm() string                                                 { return "Kill processes?" }
func (stubKill) Destructive() bool                                               { return true }
func (stubKill) Execute(context.Context, []core.ProcessInfo) []core.ActionResult { return nil }

func makeHerdFinding(n int) core.Finding {
	parent := core.ProcessInfo{PID: 1000, Command: "Chrome", CPUPercent: 1.0}
	children := make([]core.ProcessInfo, n-1)
	for i := range children {
		children[i] = core.ProcessInfo{PID: 1001 + i, Command: "Chrome Helper", CPUPercent: 1.0}
	}
	return core.Finding{
		Detector: "herd",
		Process:  parent,
		Group:    &core.HerdGroup{Parent: parent, Children: children},
	}
}

// TestConfirmFooterRevertedToFlatKeyHint locks in the rollback from the
// `⏺ yes / ⏺ no` button experiment back to the plain keyhint.
func TestConfirmFooterRevertedToFlatKeyHint(t *testing.T) {
	m := Model{
		Findings:      []core.Finding{makeHerdFinding(2)},
		Selected:      map[int]bool{0: true},
		PendingAction: stubKill{},
		Theme:         style.DefaultTheme(),
	}
	out := renderConfirmBody(m.Theme, m, 80)
	if !strings.Contains(out, "[y] confirm · [n] cancel") {
		t.Errorf("expected flat keyhint, got:\n%s", out)
	}
	if strings.Contains(out, "⏺ yes") || strings.Contains(out, "⏺ no") {
		t.Errorf("yes/no button glyphs should be removed, got:\n%s", out)
	}
}

// TestConfirmShowsScrollIndicatorsWhenOverflowing verifies that a herd-kill
// with many targets shows ↑/↓ indicators, surfaces a scroll hint, and the
// visible window matches m.ConfirmOffset.
func TestConfirmShowsScrollIndicatorsWhenOverflowing(t *testing.T) {
	m := Model{
		Findings:      []core.Finding{makeHerdFinding(30)},
		Selected:      map[int]bool{0: true},
		PendingAction: stubKill{},
		Width:         80,
		Height:        20, // forces windowing
		ConfirmOffset: 5,
		Theme:         style.DefaultTheme(),
	}
	out := renderConfirmBody(m.Theme, m, 76)

	if !strings.Contains(out, "↑ 5 more above") {
		t.Errorf("expected '↑ 5 more above' indicator, got:\n%s", out)
	}
	if !strings.Contains(out, "more below") {
		t.Errorf("expected '↓ N more below' indicator, got:\n%s", out)
	}
	if !strings.Contains(out, "[↑/↓] scroll") {
		t.Errorf("expected scroll keyhint, got:\n%s", out)
	}
}

// TestConfirmHidesIndicatorsWhenAllFit guards against drawing scroll chrome
// for tiny target lists.
func TestConfirmHidesIndicatorsWhenAllFit(t *testing.T) {
	m := Model{
		Findings: []core.Finding{
			{Process: core.ProcessInfo{PID: 1, Command: "a"}},
			{Process: core.ProcessInfo{PID: 2, Command: "b"}},
		},
		Selected:      map[int]bool{0: true, 1: true},
		PendingAction: stubKill{},
		Width:         80,
		Height:        50,
		Theme:         style.DefaultTheme(),
	}
	out := renderConfirmBody(m.Theme, m, 76)

	if strings.Contains(out, "more above") || strings.Contains(out, "more below") {
		t.Errorf("indicators should be hidden when all targets fit, got:\n%s", out)
	}
	if strings.Contains(out, "[↑/↓] scroll") {
		t.Errorf("scroll hint should be hidden when all targets fit, got:\n%s", out)
	}
}

// TestConfirmOffsetClampsAtMax verifies clampConfirmOffset prevents scrolling
// past the last full page (mirrors list-view scroll semantics).
func TestConfirmOffsetClampsAtMax(t *testing.T) {
	m := Model{
		Findings:      []core.Finding{makeHerdFinding(10)},
		Selected:      map[int]bool{0: true},
		PendingAction: stubKill{},
		Width:         80,
		Height:        20,
		ConfirmOffset: 999, // intentionally absurd
		Theme:         style.DefaultTheme(),
	}
	clamped := m.clampConfirmOffset()
	visible := clamped.confirmVisibleCount(10, false)
	if clamped.ConfirmOffset > 10-visible {
		t.Errorf("offset %d exceeds max %d (n=10, visible=%d)",
			clamped.ConfirmOffset, 10-visible, visible)
	}
	if clamped.ConfirmOffset < 0 {
		t.Errorf("offset went negative: %d", clamped.ConfirmOffset)
	}
}
