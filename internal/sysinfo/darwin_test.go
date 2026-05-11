//go:build darwin

package sysinfo

import (
	"context"
	"math"
	"os"
	"testing"
)

// TestSnapshot_NoNaNPercentages guards against gopsutil returning NaN
// CPU/Mem percentages (happens for processes too young to have a delta
// sample) leaking into the renderer / JSON / history.
func TestSnapshot_NoNaNPercentages(t *testing.T) {
	procs, err := New().Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range procs {
		if math.IsNaN(p.CPUPercent) || math.IsInf(p.CPUPercent, 0) {
			t.Errorf("PID %d (%s) has invalid CPUPercent %v", p.PID, p.Command, p.CPUPercent)
		}
		if math.IsNaN(p.MemPercent) || math.IsInf(p.MemPercent, 0) {
			t.Errorf("PID %d (%s) has invalid MemPercent %v", p.PID, p.Command, p.MemPercent)
		}
	}
}

func TestSnapshot_ReturnsCurrentProcess(t *testing.T) {
	procs, err := New().Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(procs) == 0 {
		t.Fatal("expected at least one process")
	}

	myPID := os.Getpid()
	found := false
	for _, p := range procs {
		if p.PID == myPID {
			found = true
			if p.PPID == 0 {
				t.Error("own process should have non-zero PPID")
			}
			break
		}
	}
	if !found {
		t.Errorf("own PID %d not in snapshot", myPID)
	}
}
