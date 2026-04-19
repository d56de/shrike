package detectors

import (
	"testing"
	"time"

	"github.com/d56de/shrike/internal/core"
)

func zombieConfig() core.DetectorConfig {
	return core.DetectorConfig{"min_age": 5 * time.Minute}
}

func TestZombie_FindsZombieState(t *testing.T) {
	procs := []core.ProcessInfo{
		{PID: 1, Command: "z", State: core.StateZombie, ElapsedTime: 2 * time.Hour, PPID: 1},
		{PID: 2, Command: "ok", State: core.StateRunning, ElapsedTime: 10 * time.Hour},
	}
	findings := NewZombie().Detect(procs, zombieConfig())
	if len(findings) != 1 {
		t.Fatalf("expected 1, got %d", len(findings))
	}
	if findings[0].Process.PID != 1 {
		t.Errorf("expected PID 1, got %d", findings[0].Process.PID)
	}
	if findings[0].Severity != core.SeverityHigh {
		t.Errorf("zombie severity should be High, got %v", findings[0].Severity)
	}
}

func TestZombie_FindsStoppedWithMediumSeverity(t *testing.T) {
	procs := []core.ProcessInfo{
		{PID: 3, Command: "s", State: core.StateStopped, ElapsedTime: 1 * time.Hour},
	}
	findings := NewZombie().Detect(procs, zombieConfig())
	if len(findings) != 1 {
		t.Fatalf("expected 1, got %d", len(findings))
	}
	if findings[0].Severity != core.SeverityMedium {
		t.Errorf("stopped should be Medium, got %v", findings[0].Severity)
	}
}

func TestZombie_FiltersYoungZombies(t *testing.T) {
	procs := []core.ProcessInfo{
		{PID: 1, State: core.StateZombie, ElapsedTime: 1 * time.Minute},
	}
	findings := NewZombie().Detect(procs, zombieConfig())
	if len(findings) != 0 {
		t.Error("under min_age, should be filtered")
	}
}
