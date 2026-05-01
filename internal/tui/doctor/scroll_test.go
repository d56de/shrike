package doctor

import (
	"testing"

	"github.com/d56de/shrike/internal/core"
)

// makeFindings returns n placeholder findings — enough for the windowing
// helpers to walk through.
func makeFindings(n int) []core.Finding {
	out := make([]core.Finding, n)
	for i := range out {
		out[i].Process.PID = i + 1
	}
	return out
}

func TestVisibleCountReturnsAllWhenHeightUnknown(t *testing.T) {
	m := Model{Findings: makeFindings(50)}
	if got := m.visibleCount(); got != 50 {
		t.Fatalf("Height=0 should show all findings, got %d", got)
	}
}

func TestVisibleCountAccountsForChrome(t *testing.T) {
	// Wide terminal — at large widths the footer fits on one line so chrome
	// stays at the base 20. avail = Height - 20, visible = avail/3.
	m := Model{Findings: makeFindings(50), Width: 200, Height: 50}
	if got := m.visibleCount(); got != 10 {
		t.Fatalf("Width=200 Height=50 expected 10 visible, got %d", got)
	}

	// Narrow terminal — footer wraps; visibleCount must shrink so the wrap
	// lines don't push the frame past the bottom of the screen.
	m = Model{Findings: makeFindings(50), Width: 40, Height: 28}
	got := m.visibleCount()
	if got < 1 || got > 4 {
		t.Fatalf("Width=40 Height=28: expected ≤4 visible (footer wraps), got %d", got)
	}
}

func TestVisibleCountShrinksAsFooterWraps(t *testing.T) {
	// As terminal narrows, the footer wraps into more lines, which must
	// reduce the visible-finding count monotonically (or hold steady).
	wide := Model{Findings: makeFindings(50), Width: 200, Height: 60}.visibleCount()
	narrow := Model{Findings: makeFindings(50), Width: 60, Height: 60}.visibleCount()
	if narrow > wide {
		t.Fatalf("narrow terminal should not show MORE findings than wide one (wide=%d narrow=%d)",
			wide, narrow)
	}
}

func TestWrapKeyhintBreaksOnNarrowWidth(t *testing.T) {
	segs := keyhintSegments(true)
	wide := wrapKeyhint(segs, 200)
	if len(wide) != 1 {
		t.Errorf("width=200 should fit on 1 line, got %d", len(wide))
	}
	narrow := wrapKeyhint(segs, 30)
	if len(narrow) < 2 {
		t.Errorf("width=30 should wrap to ≥2 lines, got %d", len(narrow))
	}
}

func TestVisibleCountTinyTerminal(t *testing.T) {
	// Below chrome budget — must clamp to 1 so the cursor row is at least
	// renderable.
	m := Model{Findings: makeFindings(50), Width: 40, Height: 5}
	if got := m.visibleCount(); got != 1 {
		t.Fatalf("expected 1 on tiny terminal, got %d", got)
	}
}

func TestAdjustOffsetKeepsCursorInWindow(t *testing.T) {
	// Wide terminal so the footer fits on one line — visibleCount is purely
	// a function of Height, making the offset arithmetic predictable.
	// Width=200, Height=44 → chrome=20, avail=24, visible=8. 30 findings.
	m := Model{Findings: makeFindings(30), Width: 200, Height: 44}
	visible := m.visibleCount()
	if visible != 8 {
		t.Fatalf("test setup: expected visible=8, got %d", visible)
	}

	// Cursor at top: offset stays 0.
	m.Cursor = 0
	m = m.adjustOffset()
	if m.Offset != 0 {
		t.Errorf("cursor=0 expected offset=0, got %d", m.Offset)
	}

	// Cursor still inside first page: offset stays 0.
	m.Cursor = visible - 1
	m = m.adjustOffset()
	if m.Offset != 0 {
		t.Errorf("cursor=%d (last in window) expected offset=0, got %d",
			m.Cursor, m.Offset)
	}

	// Cursor moves past the window — must scroll down by 1.
	m.Cursor = visible
	m = m.adjustOffset()
	if want := 1; m.Offset != want {
		t.Errorf("cursor=%d visible=%d expected offset=%d, got %d",
			m.Cursor, visible, want, m.Offset)
	}

	// Cursor jumps far down — clamp so cursor sits on last visible row.
	m.Cursor = 25
	m = m.adjustOffset()
	if want := 25 - visible + 1; m.Offset != want {
		t.Errorf("cursor=25 visible=%d expected offset=%d, got %d",
			visible, want, m.Offset)
	}

	// Cursor scrolls back — offset follows up.
	m.Cursor = 10
	m = m.adjustOffset()
	if want := 10; m.Offset != want {
		t.Errorf("cursor=10 expected offset=%d, got %d", want, m.Offset)
	}

	// End of list — offset clamps so the window stays full.
	m.Cursor = 29
	m = m.adjustOffset()
	if want := 30 - visible; m.Offset != want {
		t.Errorf("cursor=29 of 30 visible=%d expected offset=%d, got %d",
			visible, want, m.Offset)
	}
}

func TestAdjustOffsetNoScrollWhenEverythingFits(t *testing.T) {
	m := Model{Findings: makeFindings(2), Width: 200, Height: 50}
	m.Cursor = 1
	m = m.adjustOffset()
	if m.Offset != 0 {
		t.Errorf("findings fit in window, offset must stay 0, got %d", m.Offset)
	}
}

func TestAdjustOffsetEmptyFindings(t *testing.T) {
	m := Model{Findings: nil, Width: 40, Height: 28}
	m.Offset = 5 // stale state
	m = m.adjustOffset()
	if m.Offset != 0 {
		t.Errorf("empty findings should reset offset to 0, got %d", m.Offset)
	}
}
