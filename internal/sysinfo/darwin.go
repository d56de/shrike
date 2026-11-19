//go:build darwin

// Package sysinfo collects process snapshots on macOS.
package sysinfo

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"time"

	"github.com/d56de/shrike/internal/core"
	"github.com/shirou/gopsutil/v4/process"
)

// anonymizeUser controls whether real usernames are replaced with a placeholder.
// Enabled when SHRIKE_ANONYMIZE_USER is set to a truthy value — used for demos
// and screenshots so personal account names don't leak into shared artifacts.
var anonymizeUser = func() bool {
	switch os.Getenv("SHRIKE_ANONYMIZE_USER") {
	case "1", "true", "yes":
		return true
	}
	return false
}()

// DefaultLoadWindow is how long Snapshot waits between its two counter readings.
const DefaultLoadWindow = time.Second

// Provider implements core.Snapshotter for macOS. CPU and GPU percentages are
// measured over Window between two counter readings, not over process lifetime.
type Provider struct {
	GPU    core.GPUSnapshotter                        // nil = no GPU load
	Window time.Duration                              // 0 = DefaultLoadWindow
	Sleep  func(context.Context, time.Duration) error // nil = context-aware timer
}

// New returns a Provider with GPU sampling and the default window.
func New() Provider { return Provider{GPU: GPUProvider{}, Window: DefaultLoadWindow} }

// Snapshot returns a ProcessInfo for every visible process.
func (p Provider) Snapshot(ctx context.Context) ([]core.ProcessInfo, error) {
	procs, err := process.ProcessesWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("list processes: %w", err)
	}
	window := p.Window
	if window <= 0 {
		window = DefaultLoadWindow
	}
	sleep := p.Sleep
	if sleep == nil {
		sleep = sleepContext
	}
	a, gpuA := p.read(ctx, procs)
	if err := sleep(ctx, window); err != nil {
		return nil, fmt.Errorf("sample load window: %w", err)
	}
	b, gpuB := p.read(ctx, procs)
	if !gpuA || !gpuB {
		a.GPU, b.GPU = nil, nil
	}
	load := computeLoad(a, b, window)

	out := make([]core.ProcessInfo, 0, len(procs))
	for _, proc := range procs {
		pi, err := convert(ctx, proc)
		if err != nil {
			// Process may have died between list and read; skip silently.
			continue
		}
		l := load[pi.PID]
		pi.CPUPercent, pi.GPUPercent = l.CPUPercent, l.GPUPercent
		out = append(out, pi)
	}
	return out, nil
}

// read takes one counter reading; the bool reports whether GPU telemetry worked.
func (p Provider) read(ctx context.Context, procs []*process.Process) (loadReading, bool) {
	r := loadReading{Procs: make(map[int]procReading, len(procs))}
	for _, proc := range procs {
		started, err := proc.CreateTimeWithContext(ctx)
		if err != nil {
			continue
		}
		times, err := proc.TimesWithContext(ctx)
		if err != nil || times == nil {
			continue
		}
		r.Procs[int(proc.Pid)] = procReading{Started: started, CPUSeconds: times.User + times.System, At: time.Now()}
	}
	if p.GPU == nil {
		return r, false
	}
	sample, err := p.GPU.Snapshot(ctx)
	if err != nil {
		return r, false
	}
	r.GPUAt = sample.At
	for _, d := range sample.Devices {
		r.GPU = append(r.GPU, d.Clients...)
	}
	return r, true
}

func sleepContext(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func convert(ctx context.Context, p *process.Process) (core.ProcessInfo, error) {
	pi := core.ProcessInfo{PID: int(p.Pid)}

	if v, err := p.PpidWithContext(ctx); err == nil {
		pi.PPID = int(v)
	}
	if v, err := p.UsernameWithContext(ctx); err == nil {
		if anonymizeUser {
			pi.User = "user"
		} else {
			pi.User = v
		}
	}
	if v, err := p.ExeWithContext(ctx); err == nil {
		pi.FullPath = v
		pi.Command = filepath.Base(v)
	} else if name, err := p.NameWithContext(ctx); err == nil {
		pi.Command = name
	}
	if v, err := p.CmdlineSliceWithContext(ctx); err == nil {
		pi.Args = v
	}
	if v, err := p.MemoryPercentWithContext(ctx); err == nil {
		f := float64(v)
		if !math.IsNaN(f) && !math.IsInf(f, 0) {
			pi.MemPercent = f
		}
	}
	if mi, err := p.MemoryInfoWithContext(ctx); err == nil && mi != nil {
		pi.RSS = mi.RSS
		pi.VSZ = mi.VMS
	}
	if createdMS, err := p.CreateTimeWithContext(ctx); err == nil {
		pi.StartedAt = time.UnixMilli(createdMS)
		pi.ElapsedTime = time.Since(pi.StartedAt)
	}
	if v, err := p.StatusWithContext(ctx); err == nil && len(v) > 0 {
		pi.State = stateFromStatus(v[0])
	}
	if v, err := p.NiceWithContext(ctx); err == nil {
		pi.Nice = int(v)
	}

	return pi, nil
}

func stateFromStatus(s string) core.ProcessState {
	switch s {
	case process.Running:
		return core.StateRunning
	case process.Sleep:
		return core.StateSleeping
	case process.Stop:
		return core.StateStopped
	case process.Zombie:
		return core.StateZombie
	case process.Idle:
		return core.StateIdle
	default:
		return core.StateUnknown
	}
}
