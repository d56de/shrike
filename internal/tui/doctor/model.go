// Package doctor implements the Bubble Tea one-shot doctor screen.
package doctor

import (
	"github.com/d56de/shrike/internal/core"
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
}

// NewModel constructs a doctor Model ready for Bubble Tea.
func NewModel(findings []core.Finding, actions []core.Action) Model {
	return Model{
		Findings: findings,
		Actions:  actions,
		Selected: map[int]bool{},
		Expanded: map[int]bool{},
		Mode:     ModeList,
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
