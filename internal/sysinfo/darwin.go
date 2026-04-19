//go:build darwin

// Package sysinfo collects process snapshots on macOS.
package sysinfo

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/d56de/shrike/internal/core"
	"github.com/shirou/gopsutil/v4/process"
)

// Provider implements core.Snapshotter for macOS.
type Provider struct{}

// New returns a default Provider.
func New() Provider { return Provider{} }

// Snapshot returns a ProcessInfo for every visible process.
func (Provider) Snapshot(ctx context.Context) ([]core.ProcessInfo, error) {
	procs, err := process.ProcessesWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("list processes: %w", err)
	}
	out := make([]core.ProcessInfo, 0, len(procs))
	for _, p := range procs {
		pi, err := convert(ctx, p)
		if err != nil {
			// Process may have died between list and read; skip silently.
			continue
		}
		out = append(out, pi)
	}
	return out, nil
}

func convert(ctx context.Context, p *process.Process) (core.ProcessInfo, error) {
	pi := core.ProcessInfo{PID: int(p.Pid)}

	if v, err := p.PpidWithContext(ctx); err == nil {
		pi.PPID = int(v)
	}
	if v, err := p.UsernameWithContext(ctx); err == nil {
		pi.User = v
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
	if v, err := p.CPUPercentWithContext(ctx); err == nil {
		pi.CPUPercent = v
	}
	if v, err := p.MemoryPercentWithContext(ctx); err == nil {
		pi.MemPercent = float64(v)
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
