package sysinfo

import (
	"time"

	"github.com/d56de/shrike/internal/core"
)

// procReading is one process's cumulative CPU counter at a sample point.
// Started (gopsutil CreateTime, ms) guards against PID reuse between readings.
type procReading struct {
	Started    int64
	CPUSeconds float64
}

// loadReading is one sample point: process CPU counters plus GPU client ticks.
type loadReading struct {
	Procs map[int]procReading
	GPU   []core.GPUClient
}

// procLoad is a process's share of CPU and GPU over one window.
type procLoad struct {
	CPUPercent, GPUPercent float64
}

// computeLoad turns two readings into per-process load over window. Only
// processes present in both readings with the same start time are reported;
// anything unmatched or running backwards counts as 0, never a guess.
// GPU ticks are taken as nanoseconds of GPU time.
func computeLoad(a, b loadReading, window time.Duration) map[int]procLoad {
	out := map[int]procLoad{}
	if window <= 0 {
		return out
	}
	for pid, pb := range b.Procs {
		pa, ok := a.Procs[pid]
		if !ok || pa.Started != pb.Started {
			continue
		}
		var l procLoad
		if d := pb.CPUSeconds - pa.CPUSeconds; d > 0 {
			l.CPUPercent = d / window.Seconds() * 100
		}
		out[pid] = l
	}
	before := make(map[uint64]core.GPUClient, len(a.GPU))
	for _, c := range a.GPU {
		before[c.ID] = c
	}
	for _, c := range b.GPU {
		l, ok := out[c.PID]
		prev, seen := before[c.ID]
		if !ok || !seen || prev.PID != c.PID || prev.Channels != c.Channels || c.Ticks < prev.Ticks {
			continue
		}
		l.GPUPercent += float64(c.Ticks-prev.Ticks) / float64(window.Nanoseconds()) * 100
		out[c.PID] = l
	}
	return out
}
