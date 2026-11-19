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

// GPU detects repeated GPU overload, measured as the sum of the processes'
// GPUPercent over each scan's sampling window. The device's ioreg utilization is
// carried only as a raw display value: it reads 47–80 % on an idle M1.
type GPU struct {
	source core.GPUSnapshotter
	mu     sync.Mutex
	state  gpuHistory
}

type gpuHistory struct {
	device        uint64
	at, highSince time.Time
	samples       int
}

// NewGPU constructs a detector with an injectable telemetry source.
func NewGPU(source core.GPUSnapshotter) *GPU {
	return &GPU{source: source}
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
		g.state = gpuHistory{}
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

	// Ignored and zombie processes still count toward the load; they are only
	// kept out of the candidate list.
	var total float64
	var active []core.ProcessInfo
	for _, p := range procs {
		if p.PID <= 0 || p.GPUPercent <= 0 || math.IsNaN(p.GPUPercent) {
			continue
		}
		total += p.GPUPercent
		if !slices.Contains(ignore, p.Command) && p.State != core.StateZombie {
			active = append(active, p)
		}
	}

	d := sample.Devices[0]
	h := g.state
	if h.device != d.ID || !sample.At.After(h.at) || sample.At.Sub(h.at) > 2*time.Minute {
		h = gpuHistory{}
	}
	h.device, h.at = d.ID, sample.At
	if total < threshold {
		h.highSince, h.samples = time.Time{}, 0
		g.state = h
		return nil
	}
	if h.highSince.IsZero() {
		h.highSince = sample.At
	}
	h.samples++
	g.state = h

	elapsed := sample.At.Sub(h.highSince)
	meta := core.GPUFinding{DeviceID: d.ID, Device: d.Name, Utilization: d.Utilization, LoadPercent: total, ObservedSeconds: elapsed.Seconds(), Samples: h.samples}
	base := core.Finding{Detector: "gpu", System: true, Process: core.ProcessInfo{Command: d.Name}, GPU: &meta, Severity: core.SeverityMedium, Score: total}
	base.Reason = fmt.Sprintf("GPU %.1f%% (sum of processes); awaiting repeated observations (%d/3, %s/%s)", total, h.samples, elapsed.Round(time.Second), minDuration)
	if h.samples < 3 || elapsed < minDuration {
		return []core.Finding{base}
	}
	base.Severity = core.SeverityHigh
	base.Reason = fmt.Sprintf("GPU %.1f%% (sum of processes); high in %d observations over %s", total, h.samples, elapsed.Round(time.Second))
	out := []core.Finding{base}

	sort.Slice(active, func(i, j int) bool {
		if active[i].GPUPercent == active[j].GPUPercent {
			return active[i].PID < active[j].PID
		}
		return active[i].GPUPercent > active[j].GPUPercent
	})
	for i, p := range active {
		if i == 3 {
			break
		}
		m := meta
		m.LoadPercent = p.GPUPercent
		out = append(out, core.Finding{
			Detector: "gpu", Process: p, Severity: core.SeverityHigh,
			Score: total - float64(i+1)/10, GPU: &m,
			Reason: fmt.Sprintf("GPU-active candidate: %.1f%% GPU over the last scan window; may be legitimate work", p.GPUPercent),
		})
	}
	return out
}
