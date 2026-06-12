package detectors

import (
	"fmt"
	"slices"
	"time"

	"github.com/d56de/shrike/internal/core"
)

const (
	memleakMaxSamples    = 10
	memleakMinSamples    = 4
	memleakGrowthMB      = 100
	memleakGrowthPct     = 0.20
	memleakGrowthFloorMB = 250
	mib                  = 1024 * 1024
)

// Memleak flags processes that use a lot of memory (hog) or whose RSS grows
// steadily across scans (leak). It is STATEFUL: it accumulates a per-PID RSS
// sample history across Detect calls so growth can be observed over time.
//
// Not safe for concurrent Detect calls on the same instance. The engine calls
// each detector's Detect once per Run (one goroutine) and Runs are sequential,
// so this is safe in practice.
type Memleak struct {
	hist map[int]*pidHistory
}

type pidHistory struct {
	startedAt time.Time // from ProcessInfo.StartedAt — detects PID reuse
	rss       []uint64  // ring buffer, newest last, capped at memleakMaxSamples
}

// NewMemleak returns a ready-to-use stateful Memleak detector.
func NewMemleak() *Memleak { return &Memleak{hist: map[int]*pidHistory{}} }

// Name implements core.Detector.
func (*Memleak) Name() string { return "memleak" }

// Emoji implements core.Detector.
func (*Memleak) Emoji() string { return "🧠" }

// Detect samples each process's RSS and emits findings for hogs and leaks.
func (m *Memleak) Detect(procs []core.ProcessInfo, cfg core.DetectorConfig) []core.Finding {
	threshold, _ := cfg["rss_threshold"].(uint64)
	if threshold == 0 {
		threshold = 1024 * mib
	}
	minAge, _ := cfg["min_age"].(time.Duration)
	if minAge == 0 {
		minAge = 5 * time.Minute
	}
	ignore, _ := cfg["ignore"].([]string)

	growthFloor := uint64(memleakGrowthFloorMB) * mib
	growthBytes := uint64(memleakGrowthMB) * mib

	// Evict tracked PIDs that are no longer present (process exited).
	live := make(map[int]bool, len(procs))
	for _, p := range procs {
		live[p.PID] = true
	}
	for pid := range m.hist {
		if !live[pid] {
			delete(m.hist, pid)
		}
	}

	var out []core.Finding
	for _, p := range procs {
		if slices.Contains(ignore, p.Command) {
			delete(m.hist, p.PID) // stop tracking ignored processes
			continue
		}

		h := m.hist[p.PID]
		switch {
		case h == nil:
			if p.RSS < growthFloor {
				continue // below floor and untracked → not interesting yet
			}
			h = &pidHistory{startedAt: p.StartedAt}
			m.hist[p.PID] = h
		case !h.startedAt.Equal(p.StartedAt):
			// PID reuse: a different process now owns this PID.
			h.startedAt = p.StartedAt
			h.rss = h.rss[:0]
		}
		h.rss = append(h.rss, p.RSS)
		if len(h.rss) > memleakMaxSamples {
			h.rss = h.rss[len(h.rss)-memleakMaxSamples:]
		}

		oldest := h.rss[0]
		newest := h.rss[len(h.rss)-1]
		// Require the rise to be sustained, not a lone end-spike: at least half
		// the growth threshold must already be visible at the window midpoint.
		mid := h.rss[len(h.rss)/2]
		growing := len(h.rss) >= memleakMinSamples &&
			newest >= oldest+growthBytes &&
			float64(newest) >= float64(oldest)*(1+memleakGrowthPct) &&
			mid >= oldest+growthBytes/2

		hog := p.RSS >= threshold && p.ElapsedTime >= minAge
		leak := growing && p.RSS >= growthFloor
		if !hog && !leak {
			continue
		}

		score := float64(p.RSS) / (1 << 30) * 50
		var growth uint64
		if growing {
			growth = newest - oldest
			score += 50 + float64(growth)/(1<<30)*150
		}

		out = append(out, core.Finding{
			Process:  p,
			Detector: "memleak",
			Severity: memleakSeverity(score),
			Score:    score,
			Reason:   memleakReason(p.RSS, growing, growth),
		})
	}
	return out
}

func memleakSeverity(score float64) core.Severity {
	switch {
	case score >= 250:
		return core.SeverityCritical
	case score >= 120:
		return core.SeverityHigh
	case score >= 50:
		return core.SeverityMedium
	default:
		return core.SeverityLow
	}
}

func memleakReason(rss uint64, growing bool, growth uint64) string {
	if growing {
		return fmt.Sprintf("%s RSS, +%d MB and climbing", formatBytes(rss), growth/mib)
	}
	return fmt.Sprintf("%s RSS", formatBytes(rss))
}

// formatBytes renders bytes as "2.4 GB" at/above 1 GiB, otherwise "512 MB".
func formatBytes(b uint64) string {
	const gib = 1024 * mib
	if b >= gib {
		return fmt.Sprintf("%.1f GB", float64(b)/float64(gib))
	}
	return fmt.Sprintf("%d MB", b/mib)
}
