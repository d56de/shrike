package actions

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/d56de/shrike/internal/core"
)

// Renicer wraps the renice syscall/process so tests can mock it.
type Renicer interface {
	Renice(pid, prio int) error
}

// SystemRenicer shells out to the `renice` command-line tool.
type SystemRenicer struct{}

// Renice sets the process priority to prio for the given PID.
func (SystemRenicer) Renice(pid, prio int) error {
	cmd := exec.Command("renice", fmt.Sprintf("%d", prio), "-p", fmt.Sprintf("%d", pid)) //nolint:gosec // argv is integer prio + integer pid, no user-controlled shell input
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%w: %s", err, string(out))
	}
	return nil
}

// Renice lowers the priority of selected processes (default +10).
type Renice struct {
	Renicer  Renicer
	Priority int // default +10 (more lenient)
}

// NewRenice returns a ready-to-use Renice action with priority +10.
func NewRenice() *Renice {
	return &Renice{Renicer: SystemRenicer{}, Priority: 10}
}

// Key implements core.Action.
func (*Renice) Key() rune { return 'r' }

// Name implements core.Action.
func (*Renice) Name() string { return "renice" }

// Confirm implements core.Action.
func (*Renice) Confirm() string { return "Lower priority (renice +10) of selected processes?" }

// Destructive implements core.Action.
func (*Renice) Destructive() bool { return true }

// Execute reniceies each target and reports per-PID results.
func (r *Renice) Execute(_ context.Context, targets []core.ProcessInfo) []core.ActionResult {
	out := make([]core.ActionResult, 0, len(targets))
	for _, t := range targets {
		err := r.Renicer.Renice(t.PID, r.Priority)
		result := core.ActionResult{PID: t.PID}
		if err != nil {
			result.Err = err
			result.Message = fmt.Sprintf("renice failed: %v", err)
		} else {
			result.Message = fmt.Sprintf("nice=%+d", r.Priority)
		}
		out = append(out, result)
	}
	return out
}
