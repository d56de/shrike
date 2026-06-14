# Design: doctor visual polish (heatmap ramp + row overflow)

- **Date:** 2026-06-14
- **Status:** Approved (brainstorm complete)
- **Scope:** Three small TUI tweaks bundled into one cycle: harmonize the heatmap green ramp,
  stop the list rows overflowing on narrow terminals, and make them degrade responsively.

## 1. Heatmap ramp → "Teal → Mint" (`internal/stats/heatmap.go`)

The `shrike stats` / `[t]` trends heatmap used an ANSI-256 green ramp (`22/28/34/46`, neon)
unrelated to the doctor theme. Re-anchor it on the doctor's own greens (cursor teal `#49A281`,
checkbox mint `#7EE787`). Replace `heatmapColors`:

```go
var heatmapColors = [5]lipgloss.Color{
	lipgloss.Color("#21262d"), // 0 — no activity
	lipgloss.Color("#1e4632"), // 1 — low
	lipgloss.Color("#2e7d54"), // 2 — medium
	lipgloss.Color("#49A281"), // 3 — high (doctor cursor teal)
	lipgloss.Color("#7EE787"), // 4 — peak (doctor checkbox mint)
}
```

Single source: `stats.Render` reads `heatmapColors` for both the grid and the legend, so this
updates the standalone `shrike stats` AND the in-doctor `[t]` trends view at once. Data-only,
no behavior change.

## 2. Row clip — the correctness guarantee (`renderListBody`, `view.go`)

Each list row is built as a fixed-width `data` Sprintf (~94 visible columns: cmd 30 + PID +
6-cell CPU bar + "% CPU" + RSS + age + severity label). `frame()` only right-pads, never
truncates — so on any terminal narrower than ~96 columns (including the classic 80), the row
overflows the `│` border and the terminal clips/wraps it, cutting the right-hand fields. With
the new ANSI-tinted age field, a mid-field clip can also bleed colour.

Fix: clip the assembled `row` to `innerWidth` **before** `pad()`, ANSI-safely — the same
`lipgloss.NewStyle().MaxWidth(innerWidth).Render(row)` pattern already used for the trends
legend. This guarantees no overflow, wrap, or colour bleed at any width, and is the safety net
under §3.

## 3. Responsive row (`renderListBody`)

The row is fixed except the **command column** (left) and the **severity label** (right). On
narrow terminals, recover space so useful fields stay visible instead of being clipped:

- **Drop the severity label** when `innerWidth < 92`. It's redundant — the CPU bar is already
  tinted by severity — and it's the right-most field, the first to be clipped.
- **Shrink the command column** when narrow: `cmdW = clamp(innerWidth - 56, 14, 30)` instead of
  a fixed 30, so the right-hand fields (CPU / RSS / age) shift into view at ~80 columns. The
  `data` Sprintf uses `%-*s` with `cmdW`; herd rows truncate the command to `cmdW-4` (≥6) to
  leave room for the ` ×N` suffix.

The thresholds/constants are approximate — they only decide *where* the responsive mode kicks
in. The §2 clip is the hard correctness guarantee (it also covers the variable-width CPU/RSS
values, the herd `×N` suffix, and the `⏸ paused` tag), so the responsive math need only be
"good enough" to keep info visible at common widths.

## Testing

- **Narrow no-overflow** (the bug + regression guard): render a finding list at `Width: 80` and
  assert every line of `m.View()` has `lipgloss.Width(line) <= 80` (mirrors the trends
  `TestStats_RenderNarrowDoesNotOverflow`).
- **Responsive sev label**: the severity label text (e.g. `"High"`) is absent at `Width: 70`
  and present at `Width: 140`.
- Heatmap ramp: no test (colour values; lipgloss strips colour in the non-TTY test env).

## Files touched

| File | Change |
|------|--------|
| `internal/stats/heatmap.go` | `heatmapColors` → Teal→Mint hex ramp |
| `internal/tui/doctor/view.go` | row clip + responsive cmd/sev in `renderListBody` |
| `internal/tui/doctor/*_test.go` | narrow-overflow + responsive-sev tests |
| `CHANGELOG.md` | Unreleased entries |

## Decisions log (from brainstorm)

1. **Ramp = Variant A (Teal→Mint)**, anchoring both existing doctor greens (`#49A281` at
   level 3, `#7EE787` at peak). (vs. mint-monochrome B, soft-sage C.)
2. **Overflow = clip + responsive** (vs. clip-only): clip is the guarantee; responsive drops the
   redundant severity label and shrinks the command column on narrow terminals.
3. **Thresholds:** sev dropped below `innerWidth 92`; `cmdW = clamp(innerWidth-56, 14, 30)`.
4. Bundled and run inline (small, single-package-ish changes).
