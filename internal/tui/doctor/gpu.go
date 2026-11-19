package doctor

import (
	"fmt"

	"github.com/d56de/shrike/internal/core"
)

// gpuRow leads with measured process GPU load; the raw ioreg utilization is only
// a suffix because it reads 47–80 % on an idle M1. System warnings deliberately
// have no fabricated PID/CPU/RSS/age fields.
func gpuRow(f core.Finding, width int) string {
	name := truncate(f.Process.Command, width)
	if f.System {
		if f.GPU == nil {
			return fmt.Sprintf("%-*s  GPU unavailable", width, name)
		}
		row := fmt.Sprintf("%-*s  %.1f%% GPU · system", width, name, f.GPU.LoadPercent)
		if f.GPU.Utilization != nil {
			row += fmt.Sprintf(" (ioreg %.0f%%)", *f.GPU.Utilization)
		}
		return row
	}
	if f.GPU == nil {
		return fmt.Sprintf("%-*s  PID %d · GPU candidate", width, name, f.Process.PID)
	}
	return fmt.Sprintf("%-*s  PID %d · %.1f%% GPU", width, name, f.Process.PID, f.GPU.LoadPercent)
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
