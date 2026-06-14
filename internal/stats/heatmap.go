package stats

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// Cell glyph — two terminal columns wide so each day reads as a small
// square in most monospace fonts. Tight cells stacked together with no
// inter-column gap give the GitHub-style brick-wall look.
const (
	cellGlyph = "██"
	cellWidth = 2 // visual columns (not bytes — `█` is 3 bytes in UTF-8)
)

// heatmapColors are the 5-level green ramp used for the cells. Level 0 is
// the "no activity" colour (close to terminal background); levels 1–4 are
// progressively brighter greens. Tuned for dark terminal backgrounds — on a
// light theme they still read as "more green = more activity", just with
// less contrast.
var heatmapColors = [5]lipgloss.Color{
	lipgloss.Color("#21262d"), // 0 — no activity
	lipgloss.Color("#1e4632"), // 1 — low
	lipgloss.Color("#2e7d54"), // 2 — medium
	lipgloss.Color("#49A281"), // 3 — high (doctor cursor teal)
	lipgloss.Color("#7EE787"), // 4 — peak (doctor checkbox mint)
}

// RenderOptions tweaks the heatmap output. Zero values use sensible defaults.
type RenderOptions struct {
	// Metric selects which Day field drives the cell colour. Defaults to
	// "scans". Other accepted values: "findings", "high".
	Metric string
}

// Render returns a multi-line string with the activity heatmap, legend, and
// summary line. Days must be in chronological order (Aggregate output).
//
// Layout:
//
//	          Mar         Apr           May
//	Mon       █ █ █ █     █ █ █ █ █     █ █
//	          █ █ █ █     █ █ █ █ █     █ █
//	Wed       █ █ █ █     █ █ █ █ █     █ █
//	          █ █ █ █     █ █ █ █ █     █ █
//	Fri       █ █ █ █     █ █ █ █ █     █ █
//	          █ █ █ █     █ █ █ █ █     █ █
//	          █ █ █ █     █ █ █ █ █     █ █
//
//	Less █ █ █ █ █ More    216 scans · 91 active days · longest 28d · current 3d
func Render(days []Day, summary Summary, opts RenderOptions) string {
	if len(days) == 0 {
		return "(no activity in window)\n"
	}

	metric := opts.Metric
	if metric == "" {
		metric = "scans"
	}
	thresholds := quartileThresholds(days, metric)

	// Pad the front of the column so the first cell lands on the right
	// weekday row. weekStartIdx returns 0 (Sun) ... 6 (Sat).
	prefix := weekdayIndex(days[0].Date)
	cells := make([]Day, prefix+len(days))
	copy(cells[prefix:], days)

	// Pad the tail so the grid is a multiple of 7 columns wide.
	if rem := len(cells) % 7; rem != 0 {
		cells = append(cells, make([]Day, 7-rem)...)
	}

	weeks := len(cells) / 7

	// Build the seven weekday rows. We iterate columns then rows so each
	// row is a continuous string we can stylise per-cell.
	rows := make([]strings.Builder, 7)
	for col := 0; col < weeks; col++ {
		for row := 0; row < 7; row++ {
			idx := col*7 + row
			d := cells[idx]
			lvl := levelFor(d, metric, thresholds)
			style := lipgloss.NewStyle().Foreground(heatmapColors[lvl])
			// strings.Builder.WriteString never returns a real error.
			_, _ = rows[row].WriteString(style.Render(cellGlyph))
		}
	}

	// Compose the final string.
	var b strings.Builder
	b.WriteString(monthHeader(cells, weeks) + "\n")
	weekdayLabel := []string{"   ", "Mon", "   ", "Wed", "   ", "Fri", "   "}
	for row := 0; row < 7; row++ {
		b.WriteString(fmt.Sprintf("%s  %s\n", weekdayLabel[row], rows[row].String()))
	}
	b.WriteString("\n")
	b.WriteString(legendLine() + "    " + summaryLine(summary) + "\n")
	return b.String()
}

// levelFor maps one day's metric value to a 0–4 colour ramp index using
// quartile thresholds. Zero-activity days always return 0.
func levelFor(d Day, metric string, t [3]int) int {
	v := metricValue(d, metric)
	switch {
	case v <= 0:
		return 0
	case v <= t[0]:
		return 1
	case v <= t[1]:
		return 2
	case v <= t[2]:
		return 3
	default:
		return 4
	}
}

// metricValue returns the field of d selected by the metric name.
func metricValue(d Day, metric string) int {
	switch metric {
	case "findings":
		return d.Findings
	case "high":
		return d.HighFindings
	default:
		return d.Scans
	}
}

// quartileThresholds picks three cut-points (25, 50, 75 percentiles) of
// non-zero metric values so the colour ramp adapts to the user's activity
// scale. Returns zeros when no day has activity (legend stays single-coloured).
func quartileThresholds(days []Day, metric string) [3]int {
	values := make([]int, 0, len(days))
	for _, d := range days {
		if v := metricValue(d, metric); v > 0 {
			values = append(values, v)
		}
	}
	if len(values) == 0 {
		return [3]int{0, 0, 0}
	}
	sort.Ints(values)
	pick := func(p float64) int {
		idx := int(p * float64(len(values)-1))
		return values[idx]
	}
	return [3]int{pick(0.25), pick(0.50), pick(0.75)}
}

// weekdayIndex returns 0 for Sunday … 6 for Saturday — matches the row
// layout used in Render.
func weekdayIndex(t time.Time) int { return int(t.Weekday()) }

// monthHeader builds the top row of three-letter month abbreviations placed
// above the week column where each month first appears. Each column is
// `len(cellGlyph)` chars wide; labels span two columns (3 chars + 1 space)
// so they align cleanly with the cells below.
func monthHeader(cells []Day, weeks int) string {
	const leftPad = "     "
	cellW := cellWidth
	out := []byte(leftPad)
	prevMonth := time.Month(0)
	for col := 0; col < weeks; {
		month := time.Month(0)
		for row := 0; row < 7; row++ {
			if d := cells[col*7+row]; !d.Date.IsZero() {
				month = d.Date.Month()
				break
			}
		}
		if month != 0 && month != prevMonth && col+1 < weeks {
			label := month.String()[:3]
			// label (3) + spacer (cellW*2 - 3) → exactly 2 columns wide
			out = append(out, []byte(label)...)
			out = append(out, []byte(strings.Repeat(" ", cellW*2-3))...)
			prevMonth = month
			col += 2
			continue
		}
		if month != 0 {
			prevMonth = month
		}
		out = append(out, []byte(strings.Repeat(" ", cellW))...)
		col++
	}
	return string(out)
}

// legendLine renders the "Less █ █ █ █ █ More" scale shown under the grid.
func legendLine() string {
	var b strings.Builder
	b.WriteString("Less ")
	for _, c := range heatmapColors {
		b.WriteString(lipgloss.NewStyle().Foreground(c).Render(cellGlyph) + " ")
	}
	b.WriteString("More")
	return b.String()
}

// summaryLine renders the one-line stats summary shown to the right of the
// legend: total scans, active days, longest streak, current streak.
func summaryLine(s Summary) string {
	parts := []string{
		fmt.Sprintf("%d scans", s.TotalScans),
		fmt.Sprintf("%d findings", s.TotalFindings),
		fmt.Sprintf("%d active days", s.ActiveDays),
		fmt.Sprintf("longest %dd", s.LongestStreak),
		fmt.Sprintf("current %dd", s.CurrentStreak),
	}
	return strings.Join(parts, " · ")
}

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
