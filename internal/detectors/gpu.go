package detectors

import (
	"context"
	"fmt"
	"math"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/d56de/shrike/internal/core"
)

// GPU detects repeated device overload and reports active process candidates.
// All state belongs to the detector instance; process identities and client
// lifetimes must both match before counter deltas can be attributed.
type GPU struct {
	source  core.GPUSnapshotter
	mu      sync.Mutex
	devices map[uint64]*gpuHistory
}

type gpuHistory struct {
	at, highSince time.Time
	samples       int
	clients       map[uint64]gpuCounter
}

type gpuCounter struct {
	client  core.GPUClient
	started time.Time
}

// NewGPU constructs a detector with an injectable telemetry source.
func NewGPU(source core.GPUSnapshotter) *GPU {
	return &GPU{source: source, devices: map[uint64]*gpuHistory{}}
}

// Name implements core.Detector.
func (*GPU) Name() string { return "gpu" }

// Emoji implements core.Detector.
func (*GPU) Emoji() string { return "🎮" }

// Detect implements core.Detector for callers without a context.
func (g *GPU) Detect(p []core.ProcessInfo, cfg core.DetectorConfig) []core.Finding {
	return g.DetectContext(context.Background(), p, cfg)
}

// DetectContext lets engine cancellation reach the registry read.
func (g *GPU) DetectContext(ctx context.Context, procs []core.ProcessInfo, cfg core.DetectorConfig) []core.Finding {
	g.mu.Lock()
	defer g.mu.Unlock()
	sample, err := g.source.Snapshot(ctx)
	if err != nil || sample.At.IsZero() || len(sample.Devices) == 0 {
		g.devices = map[uint64]*gpuHistory{}
		reason := "GPU telemetry unavailable on this driver; overload detection inactive"
		if err != nil {
			reason = "GPU telemetry unavailable: " + err.Error()
		}
		return []core.Finding{{Detector: "gpu", System: true, Process: core.ProcessInfo{Command: "GPU telemetry"}, Severity: core.SeverityLow, Reason: reason}}
	}
	threshold, _ := cfg["threshold"].(float64)
	if math.IsNaN(threshold) || threshold <= 0 || threshold > 100 {
		threshold = 90
	}
	minDuration, _ := cfg["min_duration"].(time.Duration)
	if minDuration <= 0 {
		minDuration = 30 * time.Second
	}
	ignore, _ := cfg["ignore"].([]string)
	live := map[int]core.ProcessInfo{}
	for _, p := range procs {
		if p.PID > 0 {
			live[p.PID] = p
		}
	}
	next := map[uint64]*gpuHistory{}
	var out []core.Finding
	for _, d := range sample.Devices {
		h := g.devices[d.ID]
		if h == nil || !sample.At.After(h.at) || sample.At.Sub(h.at) > 2*time.Minute {
			h = &gpuHistory{}
		}
		current := &gpuHistory{at: sample.At, highSince: h.highSince, samples: h.samples, clients: map[uint64]gpuCounter{}}
		deltas := map[int]uint64{}
		for _, c := range d.Clients {
			p, exists := live[c.PID]
			if !exists || p.StartedAt.IsZero() {
				continue
			}
			current.clients[c.ID] = gpuCounter{client: c, started: p.StartedAt}
			prev, ok := h.clients[c.ID]
			if ok && prev.client.PID == c.PID && prev.client.Channels == c.Channels && prev.started.Equal(p.StartedAt) && c.Ticks > prev.client.Ticks {
				delta := c.Ticks - prev.client.Ticks
				if delta <= math.MaxUint64-deltas[c.PID] {
					deltas[c.PID] += delta
				}
			}
		}
		next[d.ID] = current
		meta := core.GPUFinding{DeviceID: d.ID, Device: d.Name, Utilization: d.Utilization}
		base := core.Finding{Detector: "gpu", System: true, Process: core.ProcessInfo{Command: d.Name}, GPU: &meta}
		if d.Utilization == nil || math.IsNaN(*d.Utilization) || *d.Utilization < 0 || *d.Utilization > 100 {
			current.highSince = time.Time{}
			current.samples = 0
			current.clients = nil
			base.Reason = "GPU utilization unavailable; overload detection inactive"
			out = append(out, base)
			continue
		}
		if *d.Utilization < threshold {
			current.highSince = time.Time{}
			current.samples = 0
			continue
		}
		if current.highSince.IsZero() {
			current.highSince = sample.At
		}
		current.samples++
		elapsed := sample.At.Sub(current.highSince)
		meta.ObservedSeconds = elapsed.Seconds()
		meta.Samples = current.samples
		base.Severity = core.SeverityMedium
		base.Score = *d.Utilization
		base.Reason = fmt.Sprintf("GPU %.1f%%; awaiting repeated observations (%d/3, %s/%s)", *d.Utilization, current.samples, elapsed.Round(time.Second), minDuration)
		confirmed := current.samples >= 3 && elapsed >= minDuration
		if confirmed {
			base.Severity = core.SeverityHigh
			base.Reason = fmt.Sprintf("GPU %.1f%%; high in %d observations over %s; cause not established", *d.Utilization, current.samples, elapsed.Round(time.Second))
		}
		out = append(out, base)
		if !confirmed {
			continue
		}
		pids := make([]int, 0, len(deltas))
		for pid := range deltas {
			if !slices.Contains(ignore, live[pid].Command) && live[pid].State != core.StateZombie {
				pids = append(pids, pid)
			}
		}
		sort.Slice(pids, func(i, j int) bool {
			if deltas[pids[i]] == deltas[pids[j]] {
				return pids[i] < pids[j]
			}
			return deltas[pids[i]] > deltas[pids[j]]
		})
		for i, pid := range pids {
			if i == 3 {
				break
			}
			m := meta
			m.DeltaTicks = deltas[pid]
			m.IntervalSeconds = sample.At.Sub(h.at).Seconds()
			out = append(out, core.Finding{
				Detector: "gpu", Process: live[pid], Severity: core.SeverityHigh,
				Score: *d.Utilization - float64(i+1)/10, GPU: &m,
				Reason: fmt.Sprintf("GPU-active candidate: +%d GPU-time ticks in %.1fs; may be legitimate work", m.DeltaTicks, m.IntervalSeconds),
			})
		}
	}
	g.devices = next
	return out
}
