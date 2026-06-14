# Doctor Visual Polish Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Harmonize the heatmap green ramp with the doctor theme, and stop the list rows overflowing on narrow terminals (clip + responsive).

**Architecture:** A data-only swap of `stats.heatmapColors`; in `renderListBody`, clip each row to `innerWidth` (ANSI-safe) as the correctness guarantee, plus a responsive command-column width and a severity-label drop on narrow terminals.

**Tech Stack:** Go 1.26, Bubble Tea, lipgloss, standard `go test`.

**Spec:** `docs/superpowers/specs/2026-06-14-doctor-visual-polish-design.md`

**Conventions:** `gofmt`, then `go test ./...`. Commits use `feat:`/`fix:`/`docs:`. No `Co-Authored-By` trailer.

**Key facts:** `internal/stats/heatmap.go` has `var heatmapColors = [5]lipgloss.Color{...}` (ANSI `236/22/28/34/46`), read by both grid and legend. `internal/tui/doctor/view.go` imports `lipgloss`; `renderListBody` receives `innerWidth`; `pad(s)` prepends 2 spaces; `truncate(s, n)` truncates a plain string; the row is built in the loop and written via `b.WriteString(pad(row) + "\n")`.

---

## Task 1: Heatmap ramp → Teal→Mint

**Files:**
- Modify: `internal/stats/heatmap.go`

- [ ] **Step 1: Replace `heatmapColors`**

The current declaration:

```go
var heatmapColors = [5]lipgloss.Color{
	lipgloss.Color("236"), // 0 — no activity
	lipgloss.Color("22"),  // 1 — low
	lipgloss.Color("28"),  // 2 — medium
	lipgloss.Color("34"),  // 3 — high
	lipgloss.Color("46"),  // 4 — peak
}
```

Replace with (anchored on the doctor cursor teal + checkbox mint):

```go
var heatmapColors = [5]lipgloss.Color{
	lipgloss.Color("#21262d"), // 0 — no activity
	lipgloss.Color("#1e4632"), // 1 — low
	lipgloss.Color("#2e7d54"), // 2 — medium
	lipgloss.Color("#49A281"), // 3 — high (doctor cursor teal)
	lipgloss.Color("#7EE787"), // 4 — peak (doctor checkbox mint)
}
```

- [ ] **Step 2: Build + tests (data-only; existing stats tests stay green)**

`go build ./... && go test ./internal/stats/ -count=1`

- [ ] **Step 3: Commit**

```bash
gofmt -w internal/stats/heatmap.go
git add internal/stats/heatmap.go
git commit -m "feat(stats): harmonize heatmap ramp with doctor greens (teal→mint)"
```

---

## Task 2: Row clip + responsive layout

**Files:**
- Modify: `internal/tui/doctor/view.go` (`renderListBody`)
- Test: `internal/tui/doctor/row_layout_test.go` (new)

- [ ] **Step 1: Write the failing tests**

Create `internal/tui/doctor/row_layout_test.go`:

```go
package doctor

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/d56de/shrike/internal/core"
)

func listModel(width int) Model {
	return Model{
		Width:  width,
		Height: 40,
		Findings: []core.Finding{{
			Detector: "runaway",
			Severity: core.SeverityHigh,
			Process:  core.ProcessInfo{PID: 4521, Command: "node-some-long-command", CPUPercent: 53, RSS: 1200 * 1024 * 1024},
		}},
		Selected:   map[int]bool{},
		Paused:     map[int]core.ProcessInfo{},
		KilledPIDs: map[int]bool{},
	}
}

func TestRow_NarrowDoesNotOverflow(t *testing.T) {
	const width = 80
	out := listModel(width).View()
	for _, line := range strings.Split(out, "\n") {
		if w := lipgloss.Width(line); w > width {
			t.Errorf("list line exceeds terminal width %d (got %d): %q", width, w, line)
		}
	}
}

func TestRow_SeverityLabelResponsive(t *testing.T) {
	if out := listModel(70).View(); strings.Contains(out, "High") {
		t.Errorf("expected severity label dropped on a narrow (70-col) terminal:\n%s", out)
	}
	if out := listModel(140).View(); !strings.Contains(out, "High") {
		t.Error("expected severity label shown on a wide (140-col) terminal")
	}
}
```

- [ ] **Step 2: Run, verify FAIL (the row overflows at 80; severity shows at 70)**

`go test ./internal/tui/doctor/ -run TestRow -v`

- [ ] **Step 3: Add the responsive-width block**

In `renderListBody`, right after the line `sev := t.Subtle.Render(sevLabel)`, insert:

```go
		// Responsive widths: the row is fixed except the command column (left)
		// and the trailing severity label (right). Shrink the command column and
		// drop the (redundant — the bar is severity-tinted) severity label on
		// narrow terminals so the right-hand fields stay visible. The clip below
		// is the hard guarantee; these are best-effort.
		cmdW := 30
		sevShown := innerWidth >= 92
		if !sevShown {
			cmdW = innerWidth - 56
			if cmdW < 14 {
				cmdW = 14
			}
			if cmdW > 30 {
				cmdW = 30
			}
		}
```

- [ ] **Step 4: Use `cmdW` for the command column**

The current command-column code is:

```go
		cmdLabel := truncate(f.Process.Command, 30)
		cpuPrefix := " "
		if f.Group != nil {
			cpu = f.Group.TotalCPU
			rss = f.Group.TotalRSS
			cmdLabel = truncate(f.Process.Command, 26) + fmt.Sprintf(" ×%d", len(f.Group.Children)+1)
			cpuPrefix = t.Subtle.Render("Σ")
		}
```

Replace with:

```go
		cmdLabel := truncate(f.Process.Command, cmdW)
		cpuPrefix := " "
		if f.Group != nil {
			cpu = f.Group.TotalCPU
			rss = f.Group.TotalRSS
			herdW := cmdW - 4
			if herdW < 6 {
				herdW = 6
			}
			cmdLabel = truncate(f.Process.Command, herdW) + fmt.Sprintf(" ×%d", len(f.Group.Children)+1)
			cpuPrefix = t.Subtle.Render("Σ")
		}
```

- [ ] **Step 5: Make the `data` Sprintf width-aware + conditional severity**

The current `data` assembly is:

```go
		data := fmt.Sprintf("%-30s  PID %-6d %s %s%.1f%% CPU · %-7s · %s %s",
			cmdLabel, f.Process.PID, bar, cpuPrefix, cpu, rssLabel, ageStr, sev)
```

Replace with (cmd width via `%-*s`; severity appended only when shown):

```go
		data := fmt.Sprintf("%-*s  PID %-6d %s %s%.1f%% CPU · %-7s · %s",
			cmdW, cmdLabel, f.Process.PID, bar, cpuPrefix, cpu, rssLabel, ageStr)
		if sevShown {
			data += " " + sev
		}
```

- [ ] **Step 6: Clip the row to the frame width**

The current row write is:

```go
		row := fmt.Sprintf("%s  %s %s", cursor, box, data)
		b.WriteString(pad(row) + "\n")
```

Replace with (ANSI-safe clip — no overflow, wrap, or colour bleed at any width):

```go
		row := fmt.Sprintf("%s  %s %s", cursor, box, data)
		row = lipgloss.NewStyle().MaxWidth(innerWidth).Render(row)
		b.WriteString(pad(row) + "\n")
```

- [ ] **Step 7: Run the tests, verify pass**

`go test ./internal/tui/doctor/ -run TestRow -v` (2 tests pass)

- [ ] **Step 8: Full doctor suite with the race detector**

`go test ./internal/tui/doctor/ -race -count=1`
Expected: PASS — existing scroll / confirm-scroll / pause-ignore / stats / age-heat tests stay green.

- [ ] **Step 9: Commit**

```bash
gofmt -w internal/tui/doctor/view.go internal/tui/doctor/row_layout_test.go
git add internal/tui/doctor/view.go internal/tui/doctor/row_layout_test.go
git commit -m "fix(doctor): clip + responsively shrink list rows on narrow terminals"
```

---

## Task 3: Changelog + final verification

**Files:**
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Build + full race suite + vet + lint**

```
go build ./...
go test ./... -race -count=1
go vet ./...
golangci-lint run ./...
```

Expected: build clean; all packages pass; `go vet` silent; `golangci-lint` `0 issues`.

- [ ] **Step 2: CHANGELOG — Unreleased**

In `CHANGELOG.md`, under `## [Unreleased]` (create `### Added` / `### Fixed` subsections as needed), append:

```markdown
### Added

- The `shrike stats` / `[t]` trends heatmap now uses a green ramp harmonized with the doctor
  theme (anchored on the cursor teal `#49A281` and checkbox mint `#7EE787`) instead of a
  standalone neon-green scale.

### Fixed

- `shrike doctor` list rows no longer overflow and get clipped on narrow terminals (including
  the classic 80 columns). Rows are clipped to the frame width (no wrap or colour bleed) and,
  when narrow, drop the redundant severity label and shrink the command column so the CPU / RSS
  / runtime fields stay visible.
```

(If `## [Unreleased]` already has an `### Added` from the age-heat change, merge the new bullet into it rather than duplicating the heading.)

- [ ] **Step 3: Commit**

```bash
git add CHANGELOG.md
git commit -m "docs: changelog for heatmap ramp + narrow-terminal row fix"
```

- [ ] **Step 4: Final verification**

```
go test ./... -race -count=1 && go vet ./... && golangci-lint run ./...
```

Expected: all green, no vet output, `0 issues`.

---

## Self-Review notes (resolved during planning)

- **Spec coverage:** ramp swap (T1), clip + responsive cmd/sev + tests (T2), changelog + verify incl. golangci-lint (T3). All spec sections mapped.
- **Clip is the guarantee:** `lipgloss.MaxWidth(innerWidth)` on the assembled row covers the variable-width CPU/RSS values, the herd `×N` suffix, the `⏸ paused` tag, and any responsive-threshold miss — so the responsive math (`cmdW = innerWidth-56`, sev below 92) only needs to be "good enough".
- **Width math sanity:** at width 80 (innerWidth 76) → sev dropped, cmdW 20, row ≈ 75 ≤ 76 (fits, clip a no-op); at width 140 → full row ~90, sev shown. The narrow test asserts every `m.View()` line ≤ width.
- **Test reality:** lipgloss strips colour in non-TTY tests, so the ramp has no unit test; the row tests assert width (`lipgloss.Width`) and the severity-label text, both colour-independent.
- **Type consistency:** `heatmapColors` array, `cmdW`/`sevShown`/`herdW` ints, `%-*s` with `cmdW`, `lipgloss.MaxWidth(innerWidth)` used consistently.
