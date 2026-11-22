package doctor

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/d56de/shrike/internal/core"
	"github.com/d56de/shrike/internal/tui/style"
)

const (
	loadSectionLines = 3 // CPU row, GPU row, blank line
	liveHistory      = 24
	liveInterval     = time.Second
)

// liveLoadMsg carries one live load sample; ok is false without a baseline.
type liveLoadMsg struct {
	point core.LoadPoint
	ok    bool
}

// liveTick samples once after liveInterval. The chain is sequential (the next
// tick is scheduled on arrival), so the sampler never runs concurrently.
func liveTick(sample func(context.Context) (core.LoadPoint, bool)) tea.Cmd {
	if sample == nil {
		return nil
	}
	return tea.Tick(liveInterval, func(time.Time) tea.Msg {
		p, ok := sample(context.Background())
		return liveLoadMsg{point: p, ok: ok}
	})
}

// pushBounded returns a new slice with v appended, keeping the last liveHistory.
func pushBounded(h []float64, v float64) []float64 {
	start := max(len(h)+1-liveHistory, 0)
	return append(append(make([]float64, 0, liveHistory), h[start:]...), v)
}

var sparkLevels = []rune("▁▂▃▄▅▆▇█")

// sparkline renders the last width values right-aligned; missing history is blank.
func sparkline(vals []float64, width int) string {
	vals = vals[max(len(vals)-width, 0):]
	var b strings.Builder
	b.WriteString(strings.Repeat(" ", width-len(vals)))
	for _, v := range vals {
		i := int(min(max(v, 0), 100) / 100 * float64(len(sparkLevels)-1))
		b.WriteRune(sparkLevels[i])
	}
	return b.String()
}

// loadSeverity tints load with the existing severity palette.
func loadSeverity(pct float64) string {
	switch {
	case pct >= 80:
		return "high"
	case pct >= 50:
		return "medium"
	}
	return "low"
}

func (m Model) hasLoad() bool { return m.Load.Procs > 0 || len(m.CPUHist) > 0 }

// renderLoadSection shows live sparklines once samples arrive, and the last
// scan's load as a bar before that.
// shortcut: GPU is summed per-client GPU time, so it reads 0% when GPU
// telemetry failed; plumb an availability flag from sysinfo if that confuses.
func renderLoadSection(t style.Theme, m Model) string {
	if !m.hasLoad() {
		return ""
	}
	cores := max(m.Cores, 1)
	row := func(label string, hist []float64, scan float64, tail string) string {
		pct, graph := scan, renderCPUBarColored(t, scan, liveHistory, loadSeverity(scan))
		if len(hist) > 0 {
			pct = hist[len(hist)-1]
			sev, ok := t.Severity[loadSeverity(pct)]
			if !ok {
				sev = t.Subtle
			}
			graph = sev.Bold(false).Render(sparkline(hist, liveHistory))
		}
		return pad(fmt.Sprintf("%s  %s %3.0f%%%s", t.Subtle.Render(label), graph, pct, t.Subtle.Render(tail))) + "\n"
	}
	coreTail := fmt.Sprintf(" · %d cores", cores)
	if cores == 1 {
		coreTail = " · 1 core"
	}
	var b strings.Builder
	b.WriteString(row("CPU", m.CPUHist, m.Load.CPUPercent/float64(cores), coreTail))
	b.WriteString(row("GPU", m.GPUHist, m.Load.GPUPercent, ""))
	b.WriteString(pad("") + "\n")
	return b.String()
}
