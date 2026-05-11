package stats

import (
	"strings"
	"testing"
	"time"
)

func TestRender_EmptyDays(t *testing.T) {
	out := Render(nil, Summary{}, RenderOptions{})
	if !strings.Contains(out, "no activity") {
		t.Errorf("empty input should render an explanatory placeholder, got:\n%s", out)
	}
}

// TestRender_LayoutAlignment guards against the original byte-vs-width bug
// (`len("██")` returning byte count). With cellWidth=2, every cell row in
// the rendered output must be exactly weeks*2 visual columns wide.
func TestRender_LayoutAlignment(t *testing.T) {
	// Build 14 consecutive days starting on a Sunday so prefix=0 and weeks=2.
	start := time.Date(2026, 5, 3, 0, 0, 0, 0, time.Local) // Sunday
	days := make([]Day, 14)
	for i := range days {
		days[i] = Day{Date: start.AddDate(0, 0, i), Scans: i + 1}
	}
	out := Render(days, Summary{From: days[0].Date, To: days[13].Date}, RenderOptions{})

	// Split into lines, drop ANSI escapes for measurement.
	lines := strings.Split(out, "\n")
	if len(lines) < 9 {
		t.Fatalf("expected ≥9 lines (header + 7 rows + legend), got %d:\n%s", len(lines), out)
	}
	// 7 data rows live at indices 1..7 (line 0 is the month header).
	for i := 1; i <= 7; i++ {
		stripped := stripANSI(lines[i])
		if len(stripped) < 5 {
			t.Errorf("row %d too short: %q", i, stripped)
			continue
		}
		// 14 days = 2 weeks → 2*2 = 4 visual cells per row.
		cells := stripped[5:]
		if got := visualWidth(cells); got != 4 {
			t.Errorf("row %d cell region width = %d, want 4: %q", i, got, cells)
		}
	}
}

// stripANSI removes CSI escape sequences from s so length checks operate on
// visual content. Good enough for the SGR-only output our renderer produces.
func stripANSI(s string) string {
	var b strings.Builder
	inEsc := false
	for _, r := range s {
		if inEsc {
			if r == 'm' {
				inEsc = false
			}
			continue
		}
		if r == 0x1b {
			inEsc = true
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// visualWidth counts runes, treating each rune as one column. `█` is a
// single column in this approximation — good enough for the test since we
// only need to verify byte-vs-column accounting.
func visualWidth(s string) int { return len([]rune(s)) }
