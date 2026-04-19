// Package detectors contains the built-in suspicion detectors.
package detectors

import (
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/d56de/shrike/internal/core"
)

// Runaway flags processes with sustained high CPU usage.
type Runaway struct{}

// NewRunaway returns a ready-to-use Runaway detector.
func NewRunaway() Runaway { return Runaway{} }

// Name implements core.Detector.
func (Runaway) Name() string { return "runaway" }

// Emoji implements core.Detector.
func (Runaway) Emoji() string { return "🔥" }

// Detect emits Findings for processes that exceed the configured CPU threshold
// for longer than the configured minimum age and are not in the ignore list.
func (Runaway) Detect(procs []core.ProcessInfo, cfg core.DetectorConfig) []core.Finding {
	threshold, _ := cfg["cpu_threshold"].(float64)
	if threshold == 0 {
		threshold = 50.0
	}
	minAge, _ := cfg["min_age"].(time.Duration)
	if minAge == 0 {
		minAge = 1 * time.Hour
	}
	ignore, _ := cfg["ignore"].([]string)

	var out []core.Finding
	for _, p := range procs {
		if p.CPUPercent < threshold {
			continue
		}
		if p.ElapsedTime < minAge {
			continue
		}
		if slices.Contains(ignore, p.Command) {
			continue
		}

		hours := p.ElapsedTime.Hours()
		score := p.CPUPercent * math.Log10(hours+10)

		out = append(out, core.Finding{
			Process:  p,
			Detector: "runaway",
			Severity: runawaySeverity(score),
			Score:    score,
			Reason:   fmt.Sprintf("%.0f%% CPU for %s", p.CPUPercent, formatElapsed(p.ElapsedTime)),
		})
	}
	return out
}

func runawaySeverity(score float64) core.Severity {
	switch {
	case score >= 200:
		return core.SeverityCritical
	case score >= 100:
		return core.SeverityHigh
	case score >= 50:
		return core.SeverityMedium
	default:
		return core.SeverityLow
	}
}

// formatElapsed turns a duration into a short string: "23h 36m", "4d 12h", "5m".
func formatElapsed(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	if days > 0 {
		return fmt.Sprintf("%dd %dh", days, hours)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, mins)
	}
	return fmt.Sprintf("%dm", mins)
}
