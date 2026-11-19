//go:build darwin

package sysinfo

import (
	"context"
	"errors"
	"math"
	"os"
	"testing"
	"time"

	"github.com/d56de/shrike/internal/core"
	"github.com/shirou/gopsutil/v4/process"
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
		if math.IsNaN(p.GPUPercent) || math.IsInf(p.GPUPercent, 0) {
			t.Errorf("PID %d (%s) has invalid GPUPercent %v", p.PID, p.Command, p.GPUPercent)
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

type fakeGPU struct {
	err error
}

func (f fakeGPU) Snapshot(context.Context) (core.GPUSample, error) {
	return core.GPUSample{}, f.err
}

func noSleep(calls *int, got *time.Duration) func(context.Context, time.Duration) error {
	return func(_ context.Context, d time.Duration) error {
		*calls++
		*got = d
		return nil
	}
}

func TestSnapshot_SleepsOnce(t *testing.T) {
	var calls int
	var got time.Duration
	p := Provider{Window: 250 * time.Millisecond, Sleep: noSleep(&calls, &got)}
	if _, err := p.Snapshot(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || got != 250*time.Millisecond {
		t.Fatalf("sleep calls=%d window=%v, want 1 call of 250ms", calls, got)
	}
}

func TestSnapshot_CancelDuringWindow(t *testing.T) {
	p := Provider{Sleep: func(context.Context, time.Duration) error { return context.Canceled }}
	if _, err := p.Snapshot(context.Background()); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestSnapshot_GPUFailureKeepsCPU(t *testing.T) {
	var calls int
	var got time.Duration
	p := Provider{GPU: fakeGPU{err: errors.New("ioreg broke")}, Sleep: noSleep(&calls, &got)}
	procs, err := p.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, pi := range procs {
		if pi.GPUPercent != 0 {
			t.Errorf("PID %d GPUPercent = %v, want 0", pi.PID, pi.GPUPercent)
		}
		found = found || pi.PID == os.Getpid()
	}
	if !found {
		t.Fatal("own PID missing")
	}
}

func TestSnapshot_MeasuresWindow(t *testing.T) {
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		for {
			select {
			case <-stop:
				return
			default:
			}
		}
	}()
	procs, err := Provider{Window: 500 * time.Millisecond}.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, pi := range procs {
		if pi.PID == os.Getpid() {
			if pi.CPUPercent < 50 {
				t.Fatalf("own CPUPercent = %.1f, want >= 50 while spinning", pi.CPUPercent)
			}
			return
		}
	}
	t.Fatal("own PID missing")
}

// gopsutil caches CreateTime per *process.Process, so the PID-reuse guard only
// works if reading B uses freshly listed process objects.
func TestSnapshot_RelistsProcessesForSecondReading(t *testing.T) {
	var lists int
	var calls int
	var got time.Duration
	p := Provider{Sleep: noSleep(&calls, &got), list: func(ctx context.Context) ([]*process.Process, error) {
		lists++
		return process.ProcessesWithContext(ctx)
	}}
	if _, err := p.Snapshot(context.Background()); err != nil {
		t.Fatal(err)
	}
	if lists != 2 {
		t.Fatalf("process list fetched %d times, want 2 (fresh objects for reading B)", lists)
	}
}
