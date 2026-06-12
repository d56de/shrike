package actions

import (
	"context"
	"syscall"

	"github.com/d56de/shrike/internal/core"
)

// Pause freezes a process with SIGSTOP or resumes it with SIGCONT, choosing
// per target from its State. It reuses the Killer interface from kill.go.
type Pause struct {
	Killer Killer
}

// NewPause returns a ready-to-use Pause action backed by a SystemKiller.
func NewPause() Pause { return Pause{Killer: SystemKiller{}} }

// Key implements core.Action.
func (Pause) Key() rune { return 'p' }

// Name implements core.Action.
func (Pause) Name() string { return "pause" }

// Confirm implements core.Action. Pause is reversible, so no confirmation.
func (Pause) Confirm() string { return "" }

// Destructive implements core.Action.
func (Pause) Destructive() bool { return false }

// Execute sends SIGCONT to stopped targets (resume) and SIGSTOP to everything
// else (pause), skipping zombies, which cannot be signalled meaningfully.
func (p Pause) Execute(_ context.Context, targets []core.ProcessInfo) []core.ActionResult {
	out := make([]core.ActionResult, 0, len(targets))
	for _, t := range targets {
		if t.State == core.StateZombie {
			out = append(out, core.ActionResult{PID: t.PID, Message: "skipped (zombie)"})
			continue
		}
		sig := syscall.SIGSTOP
		verb := "paused"
		if t.State == core.StateStopped {
			sig = syscall.SIGCONT
			verb = "resumed"
		}
		if err := p.Killer.Signal(t.PID, sig); err != nil {
			out = append(out, core.ActionResult{PID: t.PID, Err: err, Message: "not permitted"})
			continue
		}
		out = append(out, core.ActionResult{PID: t.PID, Message: verb})
	}
	return out
}
