package doctor

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/d56de/shrike/internal/core"
)

const mb = 1024 * 1024

// col is the display column of sub in line, -1 when absent.
func col(line, sub string) int {
	i := strings.Index(line, sub)
	if i < 0 {
		return -1
	}
	return lipgloss.Width(line[:i])
}

func TestColumnsAlign(t *testing.T) {
	m := listModel(100)
	m.Findings = []core.Finding{
		{Detector: "runaway", Process: core.ProcessInfo{PID: 13501, Command: "EcoFlow", CPUPercent: 11.5, RSS: 65 * mb, ElapsedTime: 97 * time.Minute}},
		{Detector: "runaway", Process: core.ProcessInfo{PID: 212, Command: "loginwindow", CPUPercent: 3.5, RSS: 23 * mb, ElapsedTime: 73 * time.Hour}},
		{Detector: "herd", Process: core.ProcessInfo{PID: 8332, Command: "bun"}, Group: &core.HerdGroup{TotalCPU: 1.2, TotalRSS: 237 * mb}},
		{Detector: "memleak", Process: core.ProcessInfo{PID: 9, Command: "Xcode", CPUPercent: 100, RSS: 2400 * mb, ElapsedTime: 5 * time.Minute}},
	}
	var rows []string
	for _, line := range strings.Split(ansi.Strip(m.View()), "\n") {
		if strings.Contains(line, "% CPU") {
			rows = append(rows, line)
		}
	}
	if len(rows) != 4 {
		t.Fatalf("want 4 rows, got %d", len(rows))
	}
	for _, c := range []string{"% CPU", "B · "} {
		want := col(rows[0], c)
		for _, r := range rows[1:] {
			if got := col(r, c); got != want {
				t.Errorf("%q at %d, want %d:\n%s\n%s", c, got, want, rows[0], r)
			}
		}
	}
	for _, s := range []string{"11.5% CPU", " 3.5% CPU", "Σ1.2% CPU", "100.0% CPU", " 65 MB", "2.3 GB", "1h 37m", "3d  1h", "0m"} {
		if !strings.Contains(strings.Join(rows, "\n"), s) {
			t.Errorf("missing %q in:\n%s", s, strings.Join(rows, "\n"))
		}
	}
}

func TestReasonLineSitsUnderName(t *testing.T) {
	m := listModel(100)
	m.Findings = []core.Finding{{Detector: "zombie", Reason: "kill parent PID 1", Process: core.ProcessInfo{PID: 5, Command: "defunct"}}}
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	for i, l := range lines {
		if strings.Contains(l, "defunct") {
			if got, want := col(lines[i+1], "╰ kill parent"), col(l, "defunct"); got != want {
				t.Errorf("reason at %d, name at %d:\n%s\n%s", got, want, l, lines[i+1])
			}
			return
		}
	}
	t.Fatal("row not found")
}
