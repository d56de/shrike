package detectors

import (
	"testing"
	"time"

	"github.com/d56de/shrike/internal/core"
)

func runawayConfig() core.DetectorConfig {
	return core.DetectorConfig{
		"cpu_threshold": 50.0,
		"min_age":       1 * time.Hour,
		"ignore":        []string{"WindowServer"},
	}
}

func TestRunaway_FindsHighCPULongRunner(t *testing.T) {
	procs := []core.ProcessInfo{
		{PID: 93187, Command: "Chrome Helper", CPUPercent: 99.1, ElapsedTime: 23 * time.Hour},
		{PID: 72820, Command: "claude", CPUPercent: 10.6, ElapsedTime: 2 * time.Minute},
	}
	d := NewRunaway()
	findings := d.Detect(procs, runawayConfig())

	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Process.PID != 93187 {
		t.Errorf("expected PID 93187, got %d", findings[0].Process.PID)
	}
	if findings[0].Severity < core.SeverityHigh {
		t.Errorf("expected High, got %v", findings[0].Severity)
	}
	if findings[0].Reason == "" {
		t.Error("expected non-empty reason")
	}
}

func TestRunaway_FiltersByCPUThreshold(t *testing.T) {
	procs := []core.ProcessInfo{
		{PID: 1, Command: "a", CPUPercent: 40.0, ElapsedTime: 5 * time.Hour},
	}
	findings := NewRunaway().Detect(procs, runawayConfig())
	if len(findings) != 0 {
		t.Errorf("expected 0 findings, got %d", len(findings))
	}
}

func TestRunaway_FiltersByMinAge(t *testing.T) {
	procs := []core.ProcessInfo{
		{PID: 1, Command: "a", CPUPercent: 99.0, ElapsedTime: 10 * time.Minute},
	}
	findings := NewRunaway().Detect(procs, runawayConfig())
	if len(findings) != 0 {
		t.Errorf("expected 0 findings, got %d", len(findings))
	}
}

func TestRunaway_RespectsIgnoreList(t *testing.T) {
	procs := []core.ProcessInfo{
		{PID: 1, Command: "WindowServer", CPUPercent: 99.0, ElapsedTime: 10 * time.Hour},
	}
	findings := NewRunaway().Detect(procs, runawayConfig())
	if len(findings) != 0 {
		t.Errorf("expected 0 findings (ignored), got %d", len(findings))
	}
}

func TestRunaway_SeverityScales(t *testing.T) {
	procs := []core.ProcessInfo{
		{PID: 1, Command: "low", CPUPercent: 55, ElapsedTime: 2 * time.Hour},   // score≈55*log10(12)=59 → Medium
		{PID: 2, Command: "high", CPUPercent: 99, ElapsedTime: 24 * time.Hour}, // score≈99*log10(34)=152 → High
	}
	findings := NewRunaway().Detect(procs, runawayConfig())
	if len(findings) != 2 {
		t.Fatalf("expected 2 findings, got %d", len(findings))
	}
	byPID := map[int]core.Finding{}
	for _, f := range findings {
		byPID[f.Process.PID] = f
	}
	if byPID[1].Severity != core.SeverityMedium {
		t.Errorf("expected Medium for PID 1, got %v", byPID[1].Severity)
	}
	if byPID[2].Severity != core.SeverityHigh {
		t.Errorf("expected High for PID 2, got %v", byPID[2].Severity)
	}
}
