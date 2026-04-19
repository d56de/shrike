package detectors

import (
	"fmt"
	"time"

	"github.com/d56de/shrike/internal/core"
)

// Zombie flags processes in State Zombie or Stopped (likely abandoned / stuck).
type Zombie struct{}

// NewZombie returns a ready-to-use Zombie detector.
func NewZombie() Zombie { return Zombie{} }

// Name implements core.Detector.
func (Zombie) Name() string { return "zombie" }

// Emoji implements core.Detector.
func (Zombie) Emoji() string { return "🧟" }

// Detect emits Findings for processes stuck in Zombie or Stopped state longer
// than the configured minimum age.
func (Zombie) Detect(procs []core.ProcessInfo, cfg core.DetectorConfig) []core.Finding {
	minAge, _ := cfg["min_age"].(time.Duration)
	if minAge == 0 {
		minAge = 5 * time.Minute
	}

	var out []core.Finding
	for _, p := range procs {
		if p.State != core.StateZombie && p.State != core.StateStopped {
			continue
		}
		if p.ElapsedTime < minAge {
			continue
		}

		var sev core.Severity
		var reason string
		if p.State == core.StateZombie {
			sev = core.SeverityHigh
			reason = fmt.Sprintf("Zombie process (parent PID %d)", p.PPID)
		} else {
			sev = core.SeverityMedium
			reason = "Stopped process"
		}

		out = append(out, core.Finding{
			Process:  p,
			Detector: "zombie",
			Severity: sev,
			Score:    p.ElapsedTime.Hours() * 10,
			Reason:   reason,
		})
	}
	return out
}
