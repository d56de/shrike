// Package doctor implements the Bubble Tea one-shot doctor screen.
package doctor

import (
	"fmt"
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
	ModeRunning
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

	// Offset is the index of the first visible finding in the list. Adjusted
	// automatically as Cursor moves so the cursor row stays inside the
	// viewport derived from Height. Zero on initial render.
	Offset int

	// Status line context (set by the caller before launching the program).
	Version      string
	ProcsScanned int
	RunDuration  time.Duration

	// Engine for in-TUI rescan (R key). Optional — rescan is disabled if nil.
	Engine         *core.Engine
	HistoryEnabled bool
	HistoryConfig  history.WriterConfig

	// Rescan progress state.
	Rescanning    bool
	ActionRunning bool
	SpinnerFrame  int

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
// cursor entry if no selection. Two transforms:
//
//   - Herd findings expand to every member of the group (parent + children).
//   - Zombie findings redirect to the parent PID: a zombie has already
//     terminated, so signalling it is a no-op. Killing the parent causes
//     init to inherit and reap the zombie entry.
//
// The final list is de-duplicated by PID so selecting several zombies that
// share a parent does not signal that parent multiple times.
func (m Model) selectedTargets() []core.ProcessInfo {
	var targets []core.ProcessInfo
	collect := func(i int) {
		if i < 0 || i >= len(m.Findings) {
			return
		}
		f := m.Findings[i]
		if f.Detector == "zombie" {
			redir := f.Process
			redir.PID = f.Process.PPID
			origName := f.Process.Command
			if origName == "" {
				origName = "<unknown>"
			}
			redir.Command = fmt.Sprintf("reap %s (PID %d) → kill parent PID %d",
				origName, f.Process.PID, f.Process.PPID)
			targets = append(targets, redir)
			return
		}
		if f.Group != nil {
			targets = append(targets, f.Group.Parent)
			targets = append(targets, f.Group.Children...)
			return
		}
		targets = append(targets, f.Process)
	}
	if len(m.Selected) > 0 {
		for i := range m.Selected {
			collect(i)
		}
	} else {
		collect(m.Cursor)
	}

	// Dedup by PID — multiple zombies often share one parent.
	seen := map[int]bool{}
	unique := targets[:0]
	for _, t := range targets {
		if seen[t.PID] {
			continue
		}
		seen[t.PID] = true
		unique = append(unique, t)
	}
	return unique
}

// visibleCount approximates how many findings fit on screen given the current
// terminal height. Returns len(Findings) when Height is unknown (no
// WindowSizeMsg yet), so the initial render shows everything before Bubble
// Tea has measured the window.
//
// Each finding renders as ~3 lines (row + path + connector); the chrome
// (border, top padding, optional logo, status header, footer divider, hints)
// is subtracted before dividing. Minimum return value is 1 so a degenerate
// terminal still shows the cursor row.
func (m Model) visibleCount() int {
	if m.Height <= 0 {
		if len(m.Findings) == 0 {
			return 1
		}
		return len(m.Findings)
	}
	inner := m.Width - 4
	if inner < 20 {
		inner = 20
	}
	chrome := 20 // logo branch (18) + 2 lines reserved for scroll indicators
	if inner < 46 {
		chrome = 13 // logo hidden on narrow terminals
	}
	// The footer wraps to multiple lines on narrow terminals — each extra
	// line steals from the entry budget. Use the long (hasHerd=true) form
	// so the chrome estimate stays conservative.
	if extra := len(wrapKeyhint(keyhintSegments(true), inner)) - 1; extra > 0 {
		chrome += extra
	}
	avail := m.Height - chrome
	if avail < 3 {
		return 1
	}
	return avail / 3
}

// adjustOffset clamps Offset so the Cursor finding remains inside the visible
// window. Call after any change to Cursor, Findings, Width, or Height.
func (m Model) adjustOffset() Model {
	n := len(m.Findings)
	if n == 0 {
		m.Offset = 0
		return m
	}
	visible := m.visibleCount()
	if visible >= n {
		m.Offset = 0
		return m
	}
	if m.Cursor < m.Offset {
		m.Offset = m.Cursor
	}
	if m.Cursor >= m.Offset+visible {
		m.Offset = m.Cursor - visible + 1
	}
	if maxOffset := n - visible; m.Offset > maxOffset {
		m.Offset = maxOffset
	}
	if m.Offset < 0 {
		m.Offset = 0
	}
	return m
}
