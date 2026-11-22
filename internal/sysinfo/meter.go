package sysinfo

import (
	"context"

	"github.com/d56de/shrike/internal/core"
	"github.com/shirou/gopsutil/v4/cpu"
)

// cpuTimes is the system-wide cumulative CPU counter, summed over all cores.
type cpuTimes struct{ busy, total float64 }

// LoadMeter samples system CPU and GPU load cheaply enough for a 1 Hz live view:
// one host CPU counter read and one ioreg call, no process walk. Not safe for
// concurrent Sample calls.
type LoadMeter struct {
	GPU core.GPUSnapshotter // nil = GPU always 0
	cpu func(context.Context) (cpuTimes, error)

	prevCPU *cpuTimes
	prevGPU *core.GPUSample
}

// NewLoadMeter returns a meter backed by gopsutil and ioreg.
func NewLoadMeter() *LoadMeter { return &LoadMeter{GPU: GPUProvider{}, cpu: hostCPU} }

func hostCPU(ctx context.Context) (cpuTimes, error) {
	ts, err := cpu.TimesWithContext(ctx, false)
	if err != nil || len(ts) == 0 {
		return cpuTimes{}, err
	}
	return cpuTimes{busy: ts[0].Total() - ts[0].Idle, total: ts[0].Total()}, nil
}

// Sample returns load since the previous call; ok is false until a baseline
// exists or when the CPU counter cannot be read.
func (m *LoadMeter) Sample(ctx context.Context) (core.LoadPoint, bool) {
	c, err := m.cpu(ctx)
	if err != nil {
		return core.LoadPoint{}, false
	}
	var g *core.GPUSample
	if m.GPU != nil {
		if s, err := m.GPU.Snapshot(ctx); err == nil {
			g = &s
		}
	}
	prevCPU, prevGPU := m.prevCPU, m.prevGPU
	m.prevCPU, m.prevGPU = &c, g
	if prevCPU == nil || c.total <= prevCPU.total {
		return core.LoadPoint{}, false
	}
	p := core.LoadPoint{CPUPercent: (c.busy - prevCPU.busy) / (c.total - prevCPU.total) * 100}
	if g != nil && prevGPU != nil && g.At.After(prevGPU.At) {
		p.GPUPercent = gpuDelta(*prevGPU, *g)
	}
	return p, true
}

// gpuDelta sums client GPU time between two samples with the same guards as
// computeLoad: unmatched, re-channelled or backwards clients count as 0.
func gpuDelta(a, b core.GPUSample) float64 {
	before := map[uint64]core.GPUClient{}
	for _, d := range a.Devices {
		for _, c := range d.Clients {
			before[c.ID] = c
		}
	}
	var ticks uint64
	for _, d := range b.Devices {
		for _, c := range d.Clients {
			prev, ok := before[c.ID]
			if !ok || prev.PID != c.PID || prev.Channels != c.Channels || c.Ticks < prev.Ticks {
				continue
			}
			ticks += c.Ticks - prev.Ticks
		}
	}
	return float64(ticks) / float64(b.At.Sub(a.At).Nanoseconds()) * 100
}
