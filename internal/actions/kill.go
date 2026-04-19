package actions

import (
	"context"
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/d56de/shrike/internal/core"
)

// Killer lets us inject a fake implementation in tests. The production
// implementation proxies to os.FindProcess.Signal and a Signal(0) liveness check.
type Killer interface {
	Signal(pid int, sig os.Signal) error
	Alive(pid int) bool
}

// SystemKiller is the production Killer backed by syscall.
type SystemKiller struct{}

// Signal sends sig to the process with the given PID.
func (SystemKiller) Signal(pid int, sig os.Signal) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Signal(sig)
}

// Alive reports whether the given PID is still a reachable process.
func (SystemKiller) Alive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Unix, FindProcess always succeeds; Signal(0) checks liveness.
	return p.Signal(syscall.Signal(0)) == nil
}

// Kill sends SIGTERM, escalating to SIGKILL after EscalateAfter if the
// process is still alive. Set Immediate=true to jump straight to SIGKILL.
type Kill struct {
	Killer        Killer
	EscalateAfter time.Duration // default 3s
	Immediate     bool          // if true: SIGKILL directly
}

// NewKill returns a ready-to-use Kill action with a SystemKiller and 3s
// escalation window.
func NewKill() *Kill {
	return &Kill{Killer: SystemKiller{}, EscalateAfter: 3 * time.Second}
}

// NewKillImmediate returns a Kill action that sends SIGKILL immediately.
func NewKillImmediate() *Kill {
	k := NewKill()
	k.Immediate = true
	return k
}

// Key implements core.Action.
func (k *Kill) Key() rune {
	if k.Immediate {
		return 'K'
	}
	return 'k'
}

// Name implements core.Action.
func (k *Kill) Name() string {
	if k.Immediate {
		return "kill-9"
	}
	return "kill"
}

// Confirm implements core.Action. Returned text is shown in the UI dialog.
func (k *Kill) Confirm() string {
	if k.Immediate {
		return "Send SIGKILL immediately? (process has no chance to clean up)"
	}
	return "Send SIGTERM, escalating to SIGKILL if still alive?"
}

// Destructive implements core.Action.
func (*Kill) Destructive() bool { return true }

// Execute sends signals to each target per the Immediate/EscalateAfter
// settings and returns per-PID results.
func (k *Kill) Execute(ctx context.Context, targets []core.ProcessInfo) []core.ActionResult {
	out := make([]core.ActionResult, 0, len(targets))
	for _, t := range targets {
		out = append(out, k.one(ctx, t))
	}
	return out
}

func (k *Kill) one(ctx context.Context, t core.ProcessInfo) core.ActionResult {
	if k.Immediate {
		if err := k.Killer.Signal(t.PID, syscall.SIGKILL); err != nil {
			return core.ActionResult{PID: t.PID, Err: err, Message: fmt.Sprintf("SIGKILL failed: %v", err)}
		}
		return core.ActionResult{PID: t.PID, Message: "SIGKILL sent"}
	}

	if err := k.Killer.Signal(t.PID, syscall.SIGTERM); err != nil {
		return core.ActionResult{PID: t.PID, Err: err, Message: fmt.Sprintf("SIGTERM failed: %v", err)}
	}

	wait := k.EscalateAfter
	if wait == 0 {
		wait = 3 * time.Second
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return core.ActionResult{PID: t.PID, Err: ctx.Err(), Message: "cancelled"}
	case <-timer.C:
	}

	if !k.Killer.Alive(t.PID) {
		return core.ActionResult{PID: t.PID, Message: "SIGTERM sent, process exited"}
	}
	if err := k.Killer.Signal(t.PID, syscall.SIGKILL); err != nil {
		return core.ActionResult{PID: t.PID, Err: err, Message: fmt.Sprintf("SIGKILL escalation failed: %v", err)}
	}
	return core.ActionResult{PID: t.PID, Message: "SIGTERM then SIGKILL"}
}
