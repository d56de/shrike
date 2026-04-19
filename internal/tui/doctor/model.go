// Package doctor implements the Bubble Tea one-shot doctor screen.
package doctor

import (
	"time"

	"github.com/d56de/shrike/internal/actions"
	"github.com/d56de/shrike/internal/core"
	"github.com/d56de/shrike/internal/history"
	"github.com/d56de/shrike/internal/tui/style"
)

// Mode distinguishes the current UI mode (list vs. modal vs. result).
type Mode int

// Mode values.
const (
	ModeList Mode = iota
	ModeConfirm
	ModeResults
	ModeInfo
	ModeSample
	ModeHelp
)

// Model is the Bubble Tea model for the doctor screen.
type Model struct {
	Findings      []core.Finding
	Cursor        int
	Selected      map[int]bool // by finding index
	Expanded      map[int]bool // herd-finding indices that are expanded
	Mode          Mode
	Actions       []core.Action
	PendingAction core.Action         // when Mode == ModeConfirm
	LastResults   []core.ActionResult // when Mode == ModeResults
	Width         int
	Height        int
	Theme         style.Theme

	// Status line context (set by the caller before launching the program).
	Version      string
	ProcsScanned int
	RunDuration  time.Duration

	// Engine for in-TUI rescan (R key). Optional — rescan is disabled if nil.
	Engine         *core.Engine
	HistoryEnabled bool
	HistoryConfig  history.WriterConfig

	// Rescan progress state.
	Rescanning bool

	// Sample state (populated by async sample goroutine).
	Sampling     bool
	SamplePID    int
	SampleStacks []actions.Stack
}

// NewModel constructs a doctor Model ready for Bubble Tea.
func NewModel(findings []core.Finding, acts []core.Action) Model {
	return NewModelWithTheme(findings, acts, style.DefaultTheme())
}

// NewModelWithTheme constructs a doctor Model with an explicit Theme (e.g.
// driven by user-supplied colours from config).
func NewModelWithTheme(findings []core.Finding, acts []core.Action, theme style.Theme) Model {
	return Model{
		Findings: findings,
		Actions:  acts,
		Selected: map[int]bool{},
		Expanded: map[int]bool{},
		Mode:     ModeList,
		Theme:    theme,
	}
}

// selectedTargets returns processes to operate on: selected entries, or the
// cursor entry if no selection.
func (m Model) selectedTargets() []core.ProcessInfo {
	var targets []core.ProcessInfo
	if len(m.Selected) > 0 {
		for i := range m.Selected {
			if i >= 0 && i < len(m.Findings) {
				targets = append(targets, m.Findings[i].Process)
			}
		}
		return targets
	}
	if m.Cursor >= 0 && m.Cursor < len(m.Findings) {
		targets = append(targets, m.Findings[m.Cursor].Process)
	}
	return targets
}
