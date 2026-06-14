# Design: elapsed-time heat coloring in the doctor list

- **Date:** 2026-06-14
- **Status:** Approved (brainstorm complete)
- **Scope:** A small TUI tweak — tint the elapsed-time field of each finding row on a duration
  "heat" ramp, so a long-running process visibly explains why its (time-weighted) severity is high.

## Motivation

The runaway severity is `CPU% × log10(hours+10)`, so a 53% process running for days can score
"high" (red) while a younger 74.9% process is "medium" (orange). The bar's *colour* encodes
severity, not raw CPU, which reads as inverted. Colouring the elapsed-time by how long the
process has run makes the time component legible at a glance.

## Design

### 1. `ageHeatLevel(d time.Duration) int` — pure, in `internal/tui/doctor/view.go`

Returns a 0–3 heat level on duration thresholds:

| level | when | meaning |
|-------|------|---------|
| 0 | `< 12h` | normal (untinted, like the cmd/PID/RSS text) |
| 1 | `≥ 12h` | amber |
| 2 | `≥ 2d`  | orange |
| 3 | `≥ 7d`  | red |

Pure function, table-tested.

### 2. `t.AgeHeat [4]lipgloss.Style` — in `internal/tui/style/theme.go`

A dedicated warm ramp, deliberately distinct from the severity palette so it reads as its own
"how long" scale, not a second severity signal:

- `AgeHeat[0]` = `lipgloss.NewStyle()` (identity — plain default foreground).
- `AgeHeat[1]` = ANSI `214` (gold/amber).
- `AgeHeat[2]` = ANSI `208` (orange).
- `AgeHeat[3]` = ANSI `160` (deep red).

Hardcoded defaults built in the theme constructor(s) (`DefaultTheme` and `FromConfig`). Not
config-exposed in v1 (YAGNI; the designer can tune the constants later).

### 3. Render in `renderListBody`

The age currently renders as a plain `%-7s` field inside the row's `data` Sprintf. To tint it
without breaking column alignment, **pad first, then style** (ANSI escapes would inflate a
`%-Ns` width):

```go
ageStr := fmt.Sprintf("%-7s", formatElapsedShort(int64(f.Process.ElapsedTime.Seconds())))
if !killed {
    ageStr = t.AgeHeat[ageHeatLevel(f.Process.ElapsedTime)].Render(ageStr)
}
```

and the `data` format swaps the age `%-7s` for `%s` (passing the pre-padded, pre-styled
`ageStr`). Killed rows skip the tint (the strikethrough already restyles the whole `data`), the
same way the `⏸ paused` tag is gated on `!killed`.

Scope: the main finding rows in `renderListBody` (all detectors — "running/lingering a long
time" is a meaningful signal everywhere). Herd-expansion child lines and the other modal
bodies are out of scope for v1.

## Testing

- `ageHeatLevel`: table-driven — `11h→0`, `12h→1`, `47h→1`, `48h→2`, `6d→2`, `7d→3`, `30d→3`.
- Render smoke: a finding with `ElapsedTime` of 8 days renders a row whose age text is present
  (e.g. contains `"8d"`). Colour ANSI is **not** asserted — lipgloss strips colour in the
  non-TTY test environment, so the heat *level* logic (above) is the real test; the wiring is a
  one-line `t.AgeHeat[level].Render(...)`.

## Files touched

| File | Change |
|------|--------|
| `internal/tui/style/theme.go` | `AgeHeat [4]lipgloss.Style` field + warm-ramp defaults |
| `internal/tui/doctor/view.go` | `ageHeatLevel` + the age-field render tweak in `renderListBody` |
| `internal/tui/doctor/*_test.go` | `ageHeatLevel` table test + render smoke |

## Decisions log (from brainstorm)

1. **Tiered heat ramp** (escalating with duration), not a single threshold — makes the
   time-weighting legible.
2. **Dedicated warm palette** (`214/208/160`), distinct from the severity colours.
3. **Thresholds 12h / 2d / 7d.**
4. **Main list rows only**, killed rows untinted, not config-exposed in v1.
