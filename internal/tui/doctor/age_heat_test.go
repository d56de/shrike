package doctor

import (
	"strings"
	"testing"
	"time"

	"github.com/d56de/shrike/internal/core"
)

func TestAgeHeatLevel(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want int
	}{
		{11 * time.Hour, 0},
		{12 * time.Hour, 1},
		{47 * time.Hour, 1},
		{48 * time.Hour, 2},
		{6 * 24 * time.Hour, 2},
		{7 * 24 * time.Hour, 3},
		{30 * 24 * time.Hour, 3},
	}
	for _, c := range cases {
		if got := ageHeatLevel(c.d); got != c.want {
			t.Errorf("ageHeatLevel(%v) = %d, want %d", c.d, got, c.want)
		}
	}
}

func TestAgeHeat_RowRendersLongAge(t *testing.T) {
	m := Model{
		Width:  120,
		Height: 40,
		Findings: []core.Finding{{
			Detector: "runaway",
			Severity: core.SeverityHigh,
			Process:  core.ProcessInfo{PID: 1, Command: "node", CPUPercent: 53, ElapsedTime: 8 * 24 * time.Hour},
		}},
		Selected:   map[int]bool{},
		Paused:     map[int]core.ProcessInfo{},
		KilledPIDs: map[int]bool{},
	}
	out := m.View()
	if !strings.Contains(out, "8d") {
		t.Errorf("expected the 8-day age in the row, got:\n%s", out)
	}
}
