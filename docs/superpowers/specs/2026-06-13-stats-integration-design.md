# Design: Stats integration into the doctor TUI (D)

- **Date:** 2026-06-13
- **Status:** Approved (brainstorm complete)
- **Feature:** D of the four-feature roadmap (A, B, C1 shipped; C2 LaunchAgent still pending).
- **Scope:** A single spec → plan → implement cycle: surface the existing `shrike stats` activity heatmap inside `shrike doctor` behind a key, plus one small width-adaptive helper in the stats package.

## Summary

Add a `[t]` (trends) key to the doctor TUI that opens the activity heatmap as a modal view
inside `shrike doctor`, reusing the existing `internal/stats` `Aggregate` + `Render`. The
heatmap adapts its week-count to the terminal width so it fits the frame. Any key returns to
the list. This makes the stats — currently a separate `shrike stats` command that's easy to
forget — a first-class, glanceable part of the main tool.

## Goals

- Reuse `stats.Aggregate` + `stats.Render` verbatim — no change to how stats are computed or
  drawn beyond a width helper.
- Fit the heatmap into the doctor frame at any reasonable terminal width (no line wrapping).
- No new model state, no engine/config/cmd change — a pure TUI addition plus one pure helper.
- Always fresh: the view reads history when opened.

## Non-Goals

- No metric toggle inside the view (v1 shows "scans"; `shrike stats --metric findings|high`
  remains for detail). 
- No interactivity beyond open/close (no scrolling/paging the heatmap, no week navigation).
- No change to the standalone `shrike stats` command.
- Not the header-strip (B) or per-finding-history (C) interpretations — those were considered
  and not chosen.

## Architecture

### 1. `ModeStats` + the `[t]` key (`internal/tui/doctor`)

- `model.go`: add `ModeStats` to the `Mode` const block, appended at the **end** of the iota
  block so existing mode values do not renumber.
- `update.go`: in `handleListKey`, add `case "t": m.Mode = ModeStats; return m, nil`. Add
  `ModeStats` to the existing "any key returns to list" group in `handleKey`
  (`case ModeResults, ModeInfo, ModeSample, ModeHelp:` → add `ModeStats`), so any key (incl.
  the global `esc`) closes the view. No new model fields.

### 2. Live render (`view.go`)

- `View()`'s `switch m.Mode` gains `case ModeStats:` → `title = "Shrike — trends"`,
  `body = renderStatsBody(t, m, innerWidth)`.
- `renderStatsBody(t style.Theme, m Model, innerWidth int) string`:
  1. `weeks := stats.WeeksForWidth(innerWidth - statsFramePad)` (see §3),
  2. `to := time.Now(); from := to.AddDate(0, 0, -7*weeks+1)` (mirrors the `shrike stats` cmd),
  3. `days, summary, err := stats.Aggregate(from, to)` — on error, render a one-line message;
  4. `body := stats.Render(days, summary, stats.RenderOptions{Metric: "scans"})`,
  5. write each line of `body` through the frame's `pad(...)` so it sits inside the frame,
     then a blank line, divider, and `pad(t.KeyHint.Render("[esc] back"))`.

`statsFramePad` accounts for the left indent `pad()` adds, so the heatmap's widest line still
fits `innerWidth`.

**Why live (no cached model state) is correct:** in `ModeStats` no spinner/auto-refresh tick
fires (those run only during rescan/sampling or in `ModeList`), so `View()` is invoked only on
real events (keypress, resize) — once or twice per opened view. `Aggregate`'s file read
(history ≤10 MB) at that cadence is negligible. This keeps the feature stateless and always
fresh and width-correct.

### 3. `stats.WeeksForWidth` — width-adaptive, pure (`internal/stats`)

The standalone command renders a fixed 13-week grid; inside the doctor frame that would wrap on
narrow terminals. Add a pure helper in `internal/stats` (which owns the heatmap layout
constants — cell width and the left label pad):

```go
// WeeksForWidth returns how many week-columns fit in `width` terminal columns,
// clamped to a readable range. Pure; used by callers embedding the heatmap in a
// fixed-width frame.
func WeeksForWidth(width int) int
```

- Computes `(width - leftPad) / cellWidth`, clamped to `[statsMinWeeks, statsMaxWeeks]` =
  `[4, 26]`.
- Lives in `internal/stats` so the layout math stays with the layout. Unit-tested directly.

No narrow-terminal fallback is needed: `View()` already refuses to render below 40 columns
(`"terminal too narrow"`), and at that floor `innerWidth` (≈36) comfortably fits the minimum
4-week grid (4×cellWidth + leftPad ≈ 13 columns). The clamp + that existing guard guarantee the
heatmap always fits the frame.

### 4. Footer + help hints

- `keyhintSegments` gains `"[t] trends"`.
- `renderHelpBody` gains a row `{"t", "activity trends (heatmap)"}`.

## Error handling

- `stats.Aggregate` error: `renderStatsBody` shows a single line
  `"trends unavailable: <err>"` inside the frame rather than failing the TUI.
- Empty/missing history: `stats.Render` already returns `"(no activity in window)"`; shown as-is.

## Testing (TDD)

- `internal/stats`: `WeeksForWidth` — wide width → clamps to 26; tiny width → clamps to 4;
  a mid width → the expected `(width-pad)/cellWidth`. Pure, table-driven.
- `internal/tui/doctor` (model): pressing `[t]` in `ModeList` sets `ModeStats`; any key (e.g.
  a rune, and `esc`) while in `ModeStats` returns to `ModeList`.
- `internal/tui/doctor` (render): with `XDG_STATE_HOME` set to a temp dir (empty history),
  `m.View()` in `ModeStats` contains `"no activity"`; with a seeded `history.jsonl` (a couple
  of `run`/`finding` JSONL lines), the output contains the heatmap legend (`"Less"`/`"More"`)
  or summary text. Width set via a `WindowSizeMsg` so the frame renders.

## Files touched

| File | Change |
|------|--------|
| `internal/stats/heatmap.go` (+ `_test.go`) | `WeeksForWidth` pure helper + `statsMinWeeks`/`statsMaxWeeks` |
| `internal/tui/doctor/model.go` | `ModeStats` const (appended) |
| `internal/tui/doctor/update.go` | `[t]` key; `ModeStats` in any-key-exit group |
| `internal/tui/doctor/view.go` | `View()` dispatch, `renderStatsBody`, footer + help hints; import `internal/stats` |
| `internal/tui/doctor/*_test.go` | model + render tests |
| `README.md`, `CHANGELOG.md` | document the `[t]` trends view |

## Decisions log (from brainstorm)

1. **Interpretation of "better integrated":** a stats view inside the doctor TUI behind `[t]`
   (vs. a header activity strip, vs. per-finding history). Chosen for highest value/effort:
   makes the existing heatmap a first-class part of the main tool.
2. **Live render, no cached state:** `View()` is called rarely in `ModeStats`, so reading
   history on render is fine and keeps the feature stateless and always fresh.
3. **Width-adaptive:** `stats.WeeksForWidth` (pure, clamped 4–26) so the heatmap fits the frame
   instead of wrapping; layout math lives in the stats package.
4. **Metric "scans", no toggle (YAGNI):** the in-doctor view is the glanceable overview; detail
   metrics stay on `shrike stats --metric`.
5. **Any key closes the view**, consistent with the info/sample/help modals.
