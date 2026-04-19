// Package actions contains the built-in actions the user can invoke on
// selected processes from the TUI.
package actions

import (
	"context"

	"github.com/d56de/shrike/internal/core"
)

// Info is a no-op action; the TUI uses it as a marker to open the info modal.
// Its Execute returns immediately with success for each target — the modal
// rendering happens in the TUI layer after Execute returns.
type Info struct{}

// NewInfo returns a ready-to-use Info action.
func NewInfo() Info { return Info{} }

// Key implements core.Action.
func (Info) Key() rune { return 'i' }

// Name implements core.Action.
func (Info) Name() string { return "info" }

// Confirm implements core.Action. Empty → no confirmation needed.
func (Info) Confirm() string { return "" }

// Destructive implements core.Action.
func (Info) Destructive() bool { return false }

// Execute implements core.Action. It returns a "shown" result per target.
func (Info) Execute(_ context.Context, targets []core.ProcessInfo) []core.ActionResult {
	out := make([]core.ActionResult, len(targets))
	for i, t := range targets {
		out[i] = core.ActionResult{PID: t.PID, Message: "shown"}
	}
	return out
}
