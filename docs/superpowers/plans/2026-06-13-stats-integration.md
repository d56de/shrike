# Stats Integration (doctor `[t]` trends) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `[t]` key to `shrike doctor` that opens the activity heatmap as a modal view inside the TUI, reusing `internal/stats`, sized to fit the terminal.

**Architecture:** A new `ModeStats` mode in the doctor TUI; `[t]` enters it and any key exits (like the info/help modals). `renderStatsBody` live-calls `stats.Aggregate` + `stats.Render` (cheap, since the view triggers no render ticks). A new pure `stats.WeeksForWidth` helper picks a week-count that fits the frame.

**Tech Stack:** Go 1.26, Bubble Tea, lipgloss, standard `go test`.

**Spec:** `docs/superpowers/specs/2026-06-13-stats-integration-design.md`

**Conventions for every commit:** run `gofmt`/`goimports`, then `go test ./...`. Commit messages use `feat:` / `test:` / `docs:`. No `Co-Authored-By` trailer (attribution disabled globally for this repo).

**Key existing facts:** in `internal/stats/heatmap.go`, `cellGlyph = "██"`, `cellWidth = 2`; the heatmap's grid starts 5 columns in (month-header `leftPad` is `"     "`, weekday rows are a 3-char label + 2 spaces). In `internal/tui/doctor/view.go`, `pad(s)` prepends 2 spaces; render-body functions receive `innerWidth = width - 4`; `View()` switches on `m.Mode` and falls to `renderListBody` in `default`. The `Mode` const block (in `model.go`) currently ends with `ModeConfirmIgnore`.

---

## Task 1: `stats.WeeksForWidth` (width-adaptive week count)

**Files:**
- Modify: `internal/stats/heatmap.go`
- Test: `internal/stats/heatmap_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/stats/heatmap_test.go`:

```go
func TestWeeksForWidth(t *testing.T) {
	cases := []struct {
		width int
		want  int
	}{
		{width: 0, want: 4},     // tiny → clamped to min
		{width: 13, want: 4},    // (13-5)/2 = 4
		{width: 10, want: 4},    // (10-5)/2 = 2 → clamped to min 4
		{width: 45, want: 20},   // (45-5)/2 = 20
		{width: 1000, want: 26}, // huge → clamped to max
	}
	for _, c := range cases {
		if got := WeeksForWidth(c.width); got != c.want {
			t.Errorf("WeeksForWidth(%d) = %d, want %d", c.width, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run it, verify FAIL (undefined: WeeksForWidth)**

`go test ./internal/stats/ -run TestWeeksForWidth -v`

- [ ] **Step 3: Add the helper to `internal/stats/heatmap.go`**

Append at the end of `internal/stats/heatmap.go`:

```go
// Heatmap grid geometry used by WeeksForWidth. statsLeftPad is the fixed left
// offset before the first cell column (month-header pad / weekday label + gap);
// cellWidth (defined above) is the visual width of one week column.
const (
	statsLeftPad  = 5
	statsMinWeeks = 4
	statsMaxWeeks = 26
)

// WeeksForWidth returns how many week-columns fit in `width` terminal columns,
// clamped to a readable range [statsMinWeeks, statsMaxWeeks]. Pure helper for
// callers embedding the heatmap in a fixed-width frame.
func WeeksForWidth(width int) int {
	weeks := (width - statsLeftPad) / cellWidth
	if weeks < statsMinWeeks {
		return statsMinWeeks
	}
	if weeks > statsMaxWeeks {
		return statsMaxWeeks
	}
	return weeks
}
```

- [ ] **Step 4: Run tests, verify pass**

`go test ./internal/stats/ -run TestWeeksForWidth -v` and `go test ./internal/stats/ -count=1`
Expected: PASS (new test + all existing stats tests).

- [ ] **Step 5: Commit**

```bash
gofmt -w internal/stats/heatmap.go internal/stats/heatmap_test.go
git add internal/stats/heatmap.go internal/stats/heatmap_test.go
git commit -m "feat(stats): add WeeksForWidth helper for width-adaptive heatmaps"
```

---

## Task 2: `ModeStats` + the `[t]` key

**Files:**
- Modify: `internal/tui/doctor/model.go` (new mode constant)
- Modify: `internal/tui/doctor/update.go` (`[t]` key + any-key-exit)
- Test: `internal/tui/doctor/stats_view_test.go` (new)

This task wires the mode transitions only. Because `ModeStats` is not yet in the `View()` switch, it falls through to `renderListBody` (harmless) until Task 3 — the model transition tests pass regardless.

- [ ] **Step 1: Write the failing test**

Create `internal/tui/doctor/stats_view_test.go`:

```go
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
```

(`keyRunes` is defined in `pause_ignore_test.go`, same package.)

- [ ] **Step 2: Run it, verify FAIL (undefined: ModeStats)**

`go test ./internal/tui/doctor/ -run TestStats_KeyOpensAndAnyKeyCloses -v`

- [ ] **Step 3: Add the `ModeStats` constant in `internal/tui/doctor/model.go`**

The `Mode` const block currently ends with `ModeConfirmIgnore`:

```go
	ModeHelp
	ModeConfirmIgnore // ignore-from-TUI confirm modal
)
```

Append `ModeStats`:

```go
	ModeHelp
	ModeConfirmIgnore // ignore-from-TUI confirm modal
	ModeStats         // activity-heatmap (trends) view
)
```

- [ ] **Step 4: Add the `[t]` key and any-key-exit in `internal/tui/doctor/update.go`**

In `handleKey`, the any-key-exit group currently reads:

```go
	case ModeResults, ModeInfo, ModeSample, ModeHelp:
		// Any key returns to list.
		m.Mode = ModeList
		return m, nil
```

Add `ModeStats` to it:

```go
	case ModeResults, ModeInfo, ModeSample, ModeHelp, ModeStats:
		// Any key returns to list.
		m.Mode = ModeList
		return m, nil
```

Then in `handleListKey`, add a `case "t":` immediately before the `default:` case (alongside the existing `case "p":` / `case "I":`):

```go
		case "t":
			m.Mode = ModeStats
			return m, nil
```

- [ ] **Step 5: Run the test, verify pass**

`go test ./internal/tui/doctor/ -run TestStats_KeyOpensAndAnyKeyCloses -v`
Expected: PASS.

- [ ] **Step 6: Run the full doctor suite (no regressions)**

`go test ./internal/tui/doctor/ -count=1`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
gofmt -w internal/tui/doctor/model.go internal/tui/doctor/update.go internal/tui/doctor/stats_view_test.go
git add internal/tui/doctor/model.go internal/tui/doctor/update.go internal/tui/doctor/stats_view_test.go
git commit -m "feat(doctor): add ModeStats and the [t] trends key"
```

---

## Task 3: Render the stats view

**Files:**
- Modify: `internal/tui/doctor/view.go` (View dispatch, `renderStatsBody`, footer + help hints, import `internal/stats`)
- Test: `internal/tui/doctor/stats_view_test.go` (add a render test)

- [ ] **Step 1: Write the failing render tests**

Append to `internal/tui/doctor/stats_view_test.go`:

```go
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
```

Update the import block of `internal/tui/doctor/stats_view_test.go` to:

```go
import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/d56de/shrike/internal/core"
)
```

- [ ] **Step 2: Run the render tests, verify FAIL (ModeStats falls back to the list body — no "no activity"/"Less")**

`go test ./internal/tui/doctor/ -run TestStats_Render -v`

- [ ] **Step 3: Add the `ModeStats` case to `View()` in `internal/tui/doctor/view.go`**

In `View()`'s `switch m.Mode`, add a case after the `case ModeHelp:` block (before `default:`):

```go
	case ModeStats:
		title = "Shrike — trends"
		body = renderStatsBody(t, m, innerWidth)
```

- [ ] **Step 4: Add the `renderStatsBody` function**

Add this function to `internal/tui/doctor/view.go` (e.g. after `renderHelpBody`):

```go
// renderStatsBody renders the activity heatmap (reusing internal/stats) sized
// to fit the frame. Read live — the trends view triggers no render ticks, so
// View() runs only on real events while it is open.
func renderStatsBody(t style.Theme, m Model, innerWidth int) string {
	weeks := stats.WeeksForWidth(innerWidth - 2) // -2 for pad()'s left margin
	to := time.Now()
	from := to.AddDate(0, 0, -7*weeks+1)

	var b strings.Builder
	b.WriteString(pad("") + "\n")
	days, summary, err := stats.Aggregate(from, to)
	if err != nil {
		b.WriteString(pad(t.Subtle.Render("trends unavailable: "+err.Error())) + "\n")
	} else {
		heatmap := stats.Render(days, summary, stats.RenderOptions{Metric: "scans"})
		for _, line := range strings.Split(strings.TrimRight(heatmap, "\n"), "\n") {
			b.WriteString(pad(line) + "\n")
		}
	}
	b.WriteString(pad("") + "\n")
	b.WriteString(footerDivider(t, innerWidth))
	b.WriteString(pad(t.KeyHint.Render("[esc] back")) + "\n")
	return b.String()
}
```

- [ ] **Step 5: Add the `stats` import to `internal/tui/doctor/view.go`**

Add to the import block:

```go
	"github.com/d56de/shrike/internal/stats"
```

(`time` and `strings` are already imported by `view.go`.)

- [ ] **Step 6: Add `[t]` to the footer keyhints**

In `keyhintSegments`, the actions `append` currently reads:

```go
	segs = append(segs,
		"[i]nfo", "[s]ample", "[k]ill", "[r]enice", "[p]ause", "[I]gnore",
		"[R] rescan")
```

Add `"[t] trends"`:

```go
	segs = append(segs,
		"[i]nfo", "[s]ample", "[k]ill", "[r]enice", "[p]ause", "[I]gnore",
		"[t] trends", "[R] rescan")
```

- [ ] **Step 7: Add `t` to the help modal**

In `renderHelpBody`, the `items` slice has a `{"R", "rescan (re-run detectors)"}` row. Add a trends row right after it:

```go
		{"R", "rescan (re-run detectors)"},
		{"t", "activity trends (heatmap)"},
```

- [ ] **Step 8: Run the render tests, verify pass**

`go test ./internal/tui/doctor/ -run TestStats_Render -v` (2 tests pass)

- [ ] **Step 9: Run the full doctor suite with the race detector**

`go test ./internal/tui/doctor/ -race -count=1`
Expected: PASS — existing scroll / confirm-scroll / autorefresh / pause-ignore tests stay green (the footer gained a segment, which the chrome math already accounts for via `keyhintSegments`).

- [ ] **Step 10: Commit**

```bash
gofmt -w internal/tui/doctor/view.go internal/tui/doctor/stats_view_test.go
git add internal/tui/doctor/view.go internal/tui/doctor/stats_view_test.go
git commit -m "feat(doctor): render the [t] trends heatmap view"
```

---

## Task 4: Wiring check, docs, final verification

**Files:**
- Modify: `README.md`, `CHANGELOG.md`

- [ ] **Step 1: Build + full race suite + vet**

```
go build ./...
go test ./... -race -count=1
go vet ./...
```

Expected: `go build` clean; all packages pass under `-race`; `go vet` silent. (No `cmd` change is needed — `[t]` is wired entirely inside the doctor TUI.)

- [ ] **Step 2: Manual smoke test**

`go run ./cmd/shrike doctor --threshold 1` → press `t` → the activity heatmap appears titled "Shrike — trends" with an `[esc] back` footer; press `esc` (or any key) → back to the findings list; the footer lists `[t] trends`; `?` help lists `t`. Press `q` to quit. (If there is no history yet, the view shows "(no activity in window)".)

- [ ] **Step 3: README — Detectors/Usage area**

In `README.md`, in the `## Navigation` or `## Actions` section (wherever the doctor keys are listed), add a line for the trends key, matching the existing style, e.g.:

```markdown
- `[t]` trends — open the activity heatmap (same data as `shrike stats`) without leaving the TUI
```

- [ ] **Step 4: CHANGELOG — Unreleased / Added**

In `CHANGELOG.md`, under `## [Unreleased]` → `### Added`, append:

```markdown
- `[t]` trends view in `shrike doctor`: opens the activity heatmap (the same data as
  `shrike stats`) inside the TUI, sized to the terminal width. Any key returns to the list.
```

- [ ] **Step 5: Commit**

```bash
git add README.md CHANGELOG.md
git commit -m "docs: document the [t] trends view"
```

- [ ] **Step 6: Final verification**

```
go test ./... -race -count=1 && go vet ./...
```

Expected: all green, no vet output.

---

## Self-Review notes (resolved during planning)

- **Spec coverage:** `stats.WeeksForWidth` (T1), `ModeStats` + `[t]` + any-key-exit (T2), `View()` dispatch + `renderStatsBody` (live `Aggregate`+`Render`) + footer/help hints (T3), docs + final verify (T4). All spec sections mapped. The dropped narrow-terminal fallback (per spec self-review) is correctly absent.
- **Width math:** `renderStatsBody` calls `WeeksForWidth(innerWidth - 2)` to account for `pad()`'s 2-space margin; `WeeksForWidth` subtracts the heatmap's own `statsLeftPad = 5` and divides by `cellWidth = 2`, clamped [4, 26]. At the doctor's 40-column floor the 4-week minimum fits, so no wrap.
- **Live render safety:** documented — no spinner/auto-refresh tick fires in `ModeStats`, so `View()`'s `stats.Aggregate` file read runs only on real events.
- **No placeholders:** every step has complete code/commands.
- **Type consistency:** `stats.WeeksForWidth(int) int`, `ModeStats`, `renderStatsBody(style.Theme, Model, int) string`, `stats.Aggregate`/`stats.Render`/`stats.RenderOptions{Metric}` used consistently across T1–T3. `keyRunes` reused from `pause_ignore_test.go`.
