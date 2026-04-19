// Package core defines the data types and interfaces used by shrike's
// detector/action engine. It must not import anything from internal/tui.
package core

import "time"

// ProcessState is the lifecycle state of a process.
type ProcessState int

// ProcessState values.
const (
	StateUnknown ProcessState = iota
	StateRunning
	StateSleeping
	StateStopped // SIGSTOP'd or ptrace'd
	StateZombie
	StateIdle
)

// String returns the lowercase textual representation of the state.
func (s ProcessState) String() string {
	switch s {
	case StateRunning:
		return "running"
	case StateSleeping:
		return "sleeping"
	case StateStopped:
		return "stopped"
	case StateZombie:
		return "zombie"
	case StateIdle:
		return "idle"
	default:
		return "unknown"
	}
}

// ProcessInfo is an immutable snapshot of one process at a point in time.
type ProcessInfo struct {
	PID         int
	PPID        int
	User        string
	Command     string // basename
	FullPath    string
	Args        []string
	CPUPercent  float64 // 0.0 – (100 * numCores)
	MemPercent  float64
	RSS         uint64 // bytes
	VSZ         uint64 // bytes
	StartedAt   time.Time
	ElapsedTime time.Duration
	State       ProcessState
	Nice        int
}
