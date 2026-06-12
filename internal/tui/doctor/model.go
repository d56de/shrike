// Package doctor implements the Bubble Tea one-shot doctor screen.
package doctor

import (
	"fmt"
	"sort"
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
	ModeConfirmIgnore // ignore-from-TUI confirm modal
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

	// ConfirmOffset is the index of the first visible target inside the
	// kill-confirm modal. Bulk-killing a herd can produce more targets than
	// fit on screen; ↑/↓ in ModeConfirm scrolls the list. Reset to 0 each
	// time ModeConfirm is entered.
	ConfirmOffset int

	// KilledPIDs records PIDs the user has successfully signalled in this
	// session. The list view renders them as crossed-out without removing
	// the row so the layout stays stable until the user requests a rescan.
	// Cleared on rescan.
	KilledPIDs map[int]bool

	// AutoRefreshInterval is the cadence at which the doctor silently
	// re-runs detectors when AutoRefreshOn is true. Zero disables.
	AutoRefreshInterval time.Duration

	// AutoRefreshOn is the runtime toggle for auto-refresh. Starts true if
	// the config provides a non-zero interval, then flips with [a].
	AutoRefreshOn bool

	// Info-modal lazy state. InfoLoading is true while FetchInfo is in flight,
	// InfoDetails carries the result, and InfoTargetPID guards against a
	// stale message overwriting fresh data when the user re-opens the modal
	// on a different cursor before the previous fetch returned.
	InfoLoading   bool
	InfoDetails   actions.InfoDetails
	InfoTargetPID int

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

	// Paused maps PIDs the user SIGSTOP'd this session to a snapshot of the
	// process (CPU zeroed). Paused processes are pinned into the findings list
	// as synthetic "paused" findings so they stay resumable even after the
	// detectors stop flagging them. Cleared per-PID on resume.
	Paused map[int]core.ProcessInfo

	// PauseAction performs SIGSTOP/SIGCONT. Injected so tests can stub it.
	PauseAction core.Action

	// IgnorePending is the finding awaiting an ignore-confirm
	// (Mode == ModeConfirmIgnore). Nil otherwise.
	IgnorePending *core.Finding

	// IgnorePath is the path to the machine-managed ignore.toml (injected by
	// the caller). Empty disables the [I] write.
	IgnorePath string
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
		Paused:   map[int]core.ProcessInfo{},
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
	if extra := len(wrapKeyhint(keyhintSegments(true, true), inner)) - 1; extra > 0 {
		chrome += extra
	}
	avail := m.Height - chrome
	if avail < 3 {
		return 1
	}
	return avail / 3
}

// confirmVisibleCount returns how many target rows fit inside the confirm
// modal given the current terminal height. Each target renders as two lines
// (the row + a gutter connector), except the last which is just the row.
// Chrome (header, divider, footer, frame, optional zombie warning, scroll
// indicators) is subtracted before dividing. Returns at least 1 so the
// modal always shows the cursor target on degenerate terminals.
func (m Model) confirmVisibleCount(totalTargets int, hasZombieWarning bool) int {
	if totalTargets <= 1 {
		return totalTargets
	}
	if m.Height <= 0 {
		return totalTargets
	}
	chrome := 11 // top border + leading blank + header + gutter + closing gutter + blank + divider + blank + footer + bottom border + safety
	if hasZombieWarning {
		chrome += 2
	}
	// Reserve 2 rows so we always have space for both scroll indicators
	// (otherwise toggling between scrolled / unscrolled re-flows the modal).
	chrome += 2
	avail := m.Height - chrome
	if avail < 1 {
		return 1
	}
	// Each target row = 2 visual lines (target + gutter), except the last
	// target which skips its trailing gutter. So N targets = 2N - 1 lines,
	// hence N_max = (lines + 1) / 2.
	n := (avail + 1) / 2
	if n < 1 {
		n = 1
	}
	if n > totalTargets {
		n = totalTargets
	}
	return n
}

// adjustConfirmOffset clamps ConfirmOffset so the visible window stays within
// the bounds of the targets list. Call when entering ModeConfirm or after
// any scroll key.
func (m Model) adjustConfirmOffset(totalTargets, visible int) Model {
	if visible >= totalTargets {
		m.ConfirmOffset = 0
		return m
	}
	if max := totalTargets - visible; m.ConfirmOffset > max {
		m.ConfirmOffset = max
	}
	if m.ConfirmOffset < 0 {
		m.ConfirmOffset = 0
	}
	return m
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

// isPaused reports whether the PID is currently SIGSTOP'd by the user.
func (m Model) isPaused(pid int) bool {
	_, ok := m.Paused[pid]
	return ok
}

// pauseTargets resolves the processes a [p] press should signal: the selected
// findings (or the cursor finding), herds expanded to all members, zombie
// findings skipped (a dead/reaped process cannot be paused). De-duplicated by
// PID. State is NOT overridden here — the caller flips already-paused PIDs to
// StateStopped so Execute sends SIGCONT.
func (m Model) pauseTargets() []core.ProcessInfo {
	var targets []core.ProcessInfo
	collect := func(i int) {
		if i < 0 || i >= len(m.Findings) {
			return
		}
		f := m.Findings[i]
		if f.Detector == "zombie" {
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

// mergePausedFindings appends a synthetic "paused" finding for every paused PID
// not already present in the findings list, so paused processes stay visible
// (and resumable) after detectors drop them. PIDs are sorted for a stable
// render order.
func (m Model) mergePausedFindings() Model {
	if len(m.Paused) == 0 {
		return m
	}
	present := map[int]bool{}
	for _, f := range m.Findings {
		present[f.Process.PID] = true
	}
	pids := make([]int, 0, len(m.Paused))
	for pid := range m.Paused {
		if !present[pid] {
			pids = append(pids, pid)
		}
	}
	sort.Ints(pids)
	for _, pid := range pids {
		m.Findings = append(m.Findings, core.Finding{
			Detector: "paused",
			Severity: core.SeverityLow,
			Process:  m.Paused[pid],
			Reason:   "paused by you",
		})
	}
	return m
}

// dropPausedFinding removes any synthetic "paused" finding for the given PID.
// Real findings for that PID (if any) are left in place.
func (m Model) dropPausedFinding(pid int) Model {
	out := m.Findings[:0]
	for _, f := range m.Findings {
		if f.Detector == "paused" && f.Process.PID == pid {
			continue
		}
		out = append(out, f)
	}
	m.Findings = out
	return m
}

// filterIgnored drops findings matching the just-ignored (detector, command)
// pair from the current view and re-clamps the cursor.
func (m Model) filterIgnored(detector, command string) Model {
	out := m.Findings[:0]
	for _, f := range m.Findings {
		if f.Detector == detector && f.Process.Command == command {
			continue
		}
		out = append(out, f)
	}
	m.Findings = out
	if m.Cursor >= len(m.Findings) {
		m.Cursor = len(m.Findings) - 1
	}
	if m.Cursor < 0 {
		m.Cursor = 0
	}
	return m
}
