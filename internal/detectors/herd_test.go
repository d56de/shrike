package detectors

import (
	"testing"
	"time"

	"github.com/d56de/shrike/internal/core"
)

func herdConfig() core.DetectorConfig {
	return core.DetectorConfig{
		"min_size":            5,
		"total_cpu_threshold": 30.0,
	}
}

func makeHerdProc(pid int, path string, cpu float64, rss uint64) core.ProcessInfo {
	return core.ProcessInfo{
		PID:         pid,
		Command:     "Figma Beta Helper",
		FullPath:    path,
		CPUPercent:  cpu,
		RSS:         rss,
		ElapsedTime: 1 * time.Hour,
	}
}

func TestHerd_GroupsByFullPath(t *testing.T) {
	path := "/Users/x/Figma Beta Helper"
	procs := []core.ProcessInfo{
		makeHerdProc(1, path, 8.0, 100),
		makeHerdProc(2, path, 7.0, 200),
		makeHerdProc(3, path, 6.0, 150),
		makeHerdProc(4, path, 5.0, 180),
		makeHerdProc(5, path, 4.0, 220),
	}
	findings := NewHerd().Detect(procs, herdConfig())
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	f := findings[0]
	if f.Group == nil {
		t.Fatal("expected non-nil Group")
	}
	if len(f.Group.Children) != 4 {
		t.Errorf("expected 4 children (5 total, 1 parent), got %d", len(f.Group.Children))
	}
	if f.Group.Parent.RSS != 220 {
		t.Errorf("parent should be highest-RSS member (220), got %d", f.Group.Parent.RSS)
	}
	// Total CPU: 8+7+6+5+4 = 30, meets threshold
	if f.Group.TotalCPU != 30.0 {
		t.Errorf("expected TotalCPU=30.0, got %v", f.Group.TotalCPU)
	}
}

func TestHerd_BelowMinSizeIgnored(t *testing.T) {
	path := "/p"
	procs := []core.ProcessInfo{
		makeHerdProc(1, path, 10, 100),
		makeHerdProc(2, path, 10, 100),
	}
	findings := NewHerd().Detect(procs, herdConfig())
	if len(findings) != 0 {
		t.Errorf("expected 0 findings under min_size (2 < 5), got %d", len(findings))
	}
}

func TestHerd_BelowCPUThresholdIgnored(t *testing.T) {
	path := "/p"
	var procs []core.ProcessInfo
	for i := 0; i < 6; i++ {
		procs = append(procs, makeHerdProc(i+1, path, 2.0, 10))
	}
	// Total CPU: 6*2.0 = 12.0, below 30.0 threshold
	findings := NewHerd().Detect(procs, herdConfig())
	if len(findings) != 0 {
		t.Errorf("expected 0 findings (total_cpu=12 < 30), got %d", len(findings))
	}
}

func TestHerd_FallsBackToCommandWhenFullPathEmpty(t *testing.T) {
	// Two groups of 5 each, identified by Command (FullPath empty for one group).
	var procs []core.ProcessInfo
	for i := 1; i <= 5; i++ {
		procs = append(procs, core.ProcessInfo{
			PID:        i,
			Command:    "shared-name",
			FullPath:   "", // empty
			CPUPercent: 10,
			RSS:        100,
		})
	}
	findings := NewHerd().Detect(procs, herdConfig())
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding grouped by Command fallback, got %d", len(findings))
	}
	if findings[0].Group == nil || len(findings[0].Group.Children) != 4 {
		t.Errorf("expected 4 children under Command fallback, got group=%+v", findings[0].Group)
	}
}
