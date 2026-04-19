package detectors

import (
	"fmt"
	"sort"

	"github.com/d56de/shrike/internal/core"
)

// Herd flags multiple instances of the same binary as an aggregated finding.
type Herd struct{}

// NewHerd returns a ready-to-use Herd detector.
func NewHerd() Herd { return Herd{} }

// Name implements core.Detector.
func (Herd) Name() string { return "herd" }

// Emoji implements core.Detector.
func (Herd) Emoji() string { return "👥" }

// Detect groups processes by FullPath (fallback: Command), then emits one
// Finding per group that meets both the size and CPU thresholds.
func (Herd) Detect(procs []core.ProcessInfo, cfg core.DetectorConfig) []core.Finding {
	minSize, _ := cfg["min_size"].(int)
	if minSize == 0 {
		minSize = 5
	}
	cpuThresh, _ := cfg["total_cpu_threshold"].(float64)
	if cpuThresh == 0 {
		cpuThresh = 30.0
	}

	groups := map[string][]core.ProcessInfo{}
	for _, p := range procs {
		key := p.FullPath
		if key == "" {
			key = p.Command
		}
		if key == "" {
			continue
		}
		groups[key] = append(groups[key], p)
	}

	var out []core.Finding
	for _, members := range groups {
		if len(members) < minSize {
			continue
		}
		var totalCPU float64
		var totalRSS uint64
		for _, m := range members {
			totalCPU += m.CPUPercent
			totalRSS += m.RSS
		}
		if totalCPU < cpuThresh {
			continue
		}

		// Sort by RSS descending; highest becomes the "parent" placeholder.
		sort.Slice(members, func(i, j int) bool {
			return members[i].RSS > members[j].RSS
		})
		parent := members[0]
		children := append([]core.ProcessInfo(nil), members[1:]...)

		score := totalCPU + float64(len(children))*2

		out = append(out, core.Finding{
			Process:  parent,
			Detector: "herd",
			Severity: runawaySeverity(totalCPU), // reuse helper from runaway.go
			Score:    score,
			Reason: fmt.Sprintf("%d %s processes using %.0f%% CPU, %d MB",
				len(members), parent.Command, totalCPU, totalRSS/1024/1024),
			Group: &core.HerdGroup{
				Parent:   parent,
				Children: children,
				TotalCPU: totalCPU,
				TotalRSS: totalRSS,
			},
		})
	}
	return out
}
