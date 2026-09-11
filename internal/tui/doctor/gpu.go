package doctor

import (
	"fmt"

	"github.com/d56de/shrike/internal/core"
)

// gpuRow labels device utilization separately from a process's raw GPU-time
// delta. System warnings deliberately have no fabricated PID/CPU/RSS/age fields.
func gpuRow(f core.Finding, width int) string {
	name := truncate(f.Process.Command, width)
	if f.System {
		if f.GPU == nil || f.GPU.Utilization == nil {
			return fmt.Sprintf("%-*s  GPU unavailable", width, name)
		}
		return fmt.Sprintf("%-*s  %.1f%% GPU · system", width, name, *f.GPU.Utilization)
	}
	if f.GPU == nil {
		return fmt.Sprintf("%-*s  PID %d · GPU candidate", width, name, f.Process.PID)
	}
	return fmt.Sprintf("%-*s  PID %d · GPU time +%d ticks / %.1fs", width, name, f.Process.PID, f.GPU.DeltaTicks, f.GPU.IntervalSeconds)
}

func findSystemIndex(findings []core.Finding, previous core.Finding) int {
	for i, f := range findings {
		if !f.System || f.Detector != previous.Detector {
			continue
		}
		if f.GPU == nil && previous.GPU == nil {
			return i
		}
		if f.GPU != nil && previous.GPU != nil && f.GPU.DeviceID == previous.GPU.DeviceID {
			return i
		}
	}
	return -1
}
