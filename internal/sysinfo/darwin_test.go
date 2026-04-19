//go:build darwin

package sysinfo

import (
	"context"
	"os"
	"testing"
)

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
