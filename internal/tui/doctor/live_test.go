package doctor

import (
	"strings"
	"testing"

	"github.com/d56de/shrike/internal/core"
)

func TestSparkline(t *testing.T) {
	if got := sparkline([]float64{0, 50, 100}, 5); got != "  ▁▄█" {
		t.Errorf("sparkline = %q", got)
	}
}

func TestLiveSampleAppendsBounded(t *testing.T) {
	m := listModel(100)
	m.Load = core.Load{Procs: 1}
	for i := range liveHistory + 5 {
		next, _ := m.Update(liveLoadMsg{point: core.LoadPoint{CPUPercent: float64(i), GPUPercent: 1}, ok: true})
		m = next.(Model)
	}
	if len(m.CPUHist) != liveHistory || m.CPUHist[liveHistory-1] != float64(liveHistory+4) {
		t.Fatalf("CPUHist len=%d last=%v", len(m.CPUHist), m.CPUHist[len(m.CPUHist)-1])
	}
	if out := m.View(); !strings.Contains(out, "▁") || !strings.Contains(out, "28%") {
		t.Errorf("expected sparkline and latest live CPU value:\n%s", out)
	}
}
