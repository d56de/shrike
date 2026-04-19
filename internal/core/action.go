package core

import "context"

// Action represents a keyed operation the user can run on selected processes.
type Action interface {
	Key() rune         // 'k'
	Name() string      // "kill"
	Confirm() string   // dialog text; empty string → no confirmation needed
	Destructive() bool // true → UI always shows confirm
	Execute(ctx context.Context, targets []ProcessInfo) []ActionResult
}

// ActionResult reports the outcome of running an Action on one process.
type ActionResult struct {
	PID     int
	Err     error
	Message string // "sent SIGTERM" | "not permitted"
}
