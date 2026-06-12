package doctor

import (
	"context"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/d56de/shrike/internal/actions"
	"github.com/d56de/shrike/internal/config"
	"github.com/d56de/shrike/internal/core"
	"github.com/d56de/shrike/internal/history"
)

// ActionMsg is emitted when an Action.Execute returns, carrying results.
type ActionMsg struct {
	Results []core.ActionResult
}

// SampleDoneMsg carries the parsed call stacks from an async sample run.
type SampleDoneMsg struct {
	PID    int
	Stacks []actions.Stack
}

// InfoDoneMsg carries the lazy-loaded extra fields shown in the info modal.
// PID is echoed back so the Update handler can discard the message when the
// user has since opened info on a different process.
type InfoDoneMsg struct {
	PID     int
	Details actions.InfoDetails
}

// RescanDoneMsg carries the result of an in-TUI rescan.
type RescanDoneMsg struct {
	Findings []core.Finding
	Duration time.Duration
	Err      error
}

// spinTickMsg advances the rescan spinner by one frame.
type spinTickMsg struct{}

// spinTick emits a spinTickMsg after the animation interval.
func spinTick() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(time.Time) tea.Msg {
		return spinTickMsg{}
	})
}

// autoRefreshTickMsg fires every AutoRefreshInterval to drive silent rescans.
type autoRefreshTickMsg struct{}

// autoRefreshTick schedules the next auto-refresh tick. Returns nil if the
// interval is non-positive — used as a no-op when auto-refresh is off.
func autoRefreshTick(interval time.Duration) tea.Cmd {
	if interval <= 0 {
		return nil
	}
	return tea.Tick(interval, func(time.Time) tea.Msg {
		return autoRefreshTickMsg{}
	})
}

// Init implements tea.Model. Kicks off the auto-refresh ticker if enabled.
func (m Model) Init() tea.Cmd {
	if m.AutoRefreshOn && m.AutoRefreshInterval > 0 {
		return autoRefreshTick(m.AutoRefreshInterval)
	}
	return nil
}

// Update implements tea.Model: dispatches based on mode.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.Width, m.Height = msg.Width, msg.Height
		return m.adjustOffset(), nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	case ActionMsg:
		m.LastResults = msg.Results
		m.Mode = ModeResults
		m.ActionRunning = false
		if m.KilledPIDs == nil {
			m.KilledPIDs = map[int]bool{}
		}
		if m.Paused == nil {
			m.Paused = map[int]core.ProcessInfo{}
		}
		for _, r := range msg.Results {
			switch r.Message {
			case "paused":
				// Already optimistically pinned in the [p] branch.
			case "resumed":
				delete(m.Paused, r.PID)
				m = m.dropPausedFinding(r.PID)
			case "skipped (zombie)":
				// no-op
			default:
				if r.Err != nil {
					// Undo the optimistic pin on a failed pause/resume; a
					// failed kill/renice simply isn't marked killed (as before).
					delete(m.Paused, r.PID)
					continue
				}
				m.KilledPIDs[r.PID] = true
			}
		}
		// Drop the bulk selection now that the action is done.
		m.Selected = map[int]bool{}
		m = m.mergePausedFindings()
		if m.Cursor >= len(m.Findings) {
			m.Cursor = len(m.Findings) - 1
		}
		if m.Cursor < 0 {
			m.Cursor = 0
		}
		return m.adjustOffset(), nil
	case SampleDoneMsg:
		m.Sampling = false
		m.SpinnerFrame = 0
		m.SamplePID = msg.PID
		m.SampleStacks = msg.Stacks
		return m, nil
	case InfoDoneMsg:
		// Discard stale message: user navigated to a different cursor and
		// re-opened info before this fetch returned. The fresh fetch is
		// already in flight; nothing to do here.
		if msg.PID != m.InfoTargetPID {
			return m, nil
		}
		m.InfoLoading = false
		m.InfoDetails = msg.Details
		return m, nil
	case RescanDoneMsg:
		m.Rescanning = false
		if m.Mode != ModeResults {
			m.SpinnerFrame = 0
		}
		if msg.Err == nil {
			// Preserve the cursor across rescans by PID so auto-refresh
			// doesn't yank focus away from whatever the user was watching.
			// Fallback: clamp the old index when the PID is gone.
			var prevPID int
			if m.Cursor >= 0 && m.Cursor < len(m.Findings) {
				prevPID = m.Findings[m.Cursor].Process.PID
			}
			m.Findings = msg.Findings
			m.RunDuration = msg.Duration
			m.Selected = map[int]bool{}
			m.Expanded = map[int]bool{}
			m.KilledPIDs = nil
			// Re-pin paused rows the rescan no longer flags so they stay
			// resumable. Done before findPIDIndex so the cursor can land on
			// a re-pinned row.
			m = m.mergePausedFindings()
			m.Cursor = findPIDIndex(m.Findings, prevPID)
			if m.Cursor < 0 {
				if m.Cursor = len(m.Findings) - 1; m.Cursor < 0 {
					m.Cursor = 0
				}
			}
		}
		return m.adjustOffset(), nil
	case spinTickMsg:
		if m.Rescanning || m.Sampling || m.ActionRunning || m.InfoLoading {
			m.SpinnerFrame++
			return m, spinTick()
		}
		return m, nil
	case autoRefreshTickMsg:
		// Drop the tick chain entirely if auto-refresh was disabled while
		// the tick was in flight. Re-enabling via [a] restarts the chain.
		if !m.AutoRefreshOn || m.AutoRefreshInterval <= 0 {
			return m, nil
		}
		// Skip the rescan when the UI is busy (modal open, action running,
		// rescan already in flight) — but keep ticking so the chain doesn't
		// die. Auto-refresh only fires from the calm idle-list state.
		busy := m.Mode != ModeList || m.Rescanning || m.ActionRunning || m.Sampling
		if busy || m.Engine == nil {
			return m, autoRefreshTick(m.AutoRefreshInterval)
		}
		m.Rescanning = true
		m.SpinnerFrame = 0
		return m, tea.Batch(
			runRescan(m.Engine, m.HistoryEnabled, m.HistoryConfig),
			spinTick(),
			autoRefreshTick(m.AutoRefreshInterval),
		)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Always-available: quit / help / escape.
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "?":
		m.Mode = ModeHelp
		return m, nil
	case "esc":
		m.Mode = ModeList
		m.IgnorePending = nil
		return m, nil
	}

	switch m.Mode {
	case ModeList:
		return m.handleListKey(msg)
	case ModeConfirm:
		return m.handleConfirmKey(msg)
	case ModeConfirmIgnore:
		return m.handleIgnoreConfirmKey(msg)
	case ModeRunning:
		// Ignore all key input while an action is in flight.
		return m, nil
	case ModeResults, ModeInfo, ModeSample, ModeHelp:
		// Any key returns to list.
		m.Mode = ModeList
		return m, nil
	}
	return m, nil
}

func (m Model) handleListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Arrow keys for navigation only — vim-style j/k/h/l would clash with the
	// [k]ill / [j] (unused) / [h]/[l] action key dispatch below.
	switch msg.String() {
	case "up":
		if m.Cursor > 0 {
			m.Cursor--
		}
	case "down":
		if m.Cursor < len(m.Findings)-1 {
			m.Cursor++
		}
	case "pgup":
		page := m.visibleCount()
		if page < 1 {
			page = 1
		}
		m.Cursor -= page
		if m.Cursor < 0 {
			m.Cursor = 0
		}
	case "pgdown":
		page := m.visibleCount()
		if page < 1 {
			page = 1
		}
		m.Cursor += page
		if last := len(m.Findings) - 1; m.Cursor > last {
			m.Cursor = last
		}
		if m.Cursor < 0 {
			m.Cursor = 0
		}
	case "home":
		m.Cursor = 0
	case "end":
		m.Cursor = len(m.Findings) - 1
		if m.Cursor < 0 {
			m.Cursor = 0
		}
	case " ":
		if m.Selected[m.Cursor] {
			delete(m.Selected, m.Cursor)
		} else {
			m.Selected[m.Cursor] = true
		}
	case "right":
		if m.Cursor < len(m.Findings) && m.Findings[m.Cursor].Group != nil {
			m.Expanded[m.Cursor] = true
		}
	case "left":
		delete(m.Expanded, m.Cursor)
	case "R":
		if m.Engine != nil && !m.Rescanning {
			m.Rescanning = true
			m.SpinnerFrame = 0
			return m, tea.Batch(runRescan(m.Engine, m.HistoryEnabled, m.HistoryConfig), spinTick())
		}
	case "a":
		// Toggle auto-refresh. If the interval is zero (never configured)
		// the toggle is a no-op — the footer hint also hides in that case,
		// so the keystroke just bounces off.
		if m.AutoRefreshInterval > 0 {
			m.AutoRefreshOn = !m.AutoRefreshOn
			if m.AutoRefreshOn {
				return m.adjustOffset(), autoRefreshTick(m.AutoRefreshInterval)
			}
		}
	case "i":
		m.Mode = ModeInfo
		if m.Cursor >= 0 && m.Cursor < len(m.Findings) {
			pid := m.Findings[m.Cursor].Process.PID
			m.InfoTargetPID = pid
			m.InfoDetails = actions.InfoDetails{}
			m.InfoLoading = true
			m.SpinnerFrame = 0
			return m, tea.Batch(runFetchInfo(pid), spinTick())
		}
	case "s":
		if m.Cursor >= 0 && m.Cursor < len(m.Findings) {
			target := m.Findings[m.Cursor].Process
			m.Mode = ModeSample
			m.Sampling = true
			m.SpinnerFrame = 0
			m.SamplePID = target.PID
			m.SampleStacks = nil
			return m, tea.Batch(runSample(target), spinTick())
		}
	case "p":
		if m.PauseAction == nil {
			return m.adjustOffset(), nil
		}
		raw := m.pauseTargets()
		if len(raw) == 0 {
			return m.adjustOffset(), nil
		}
		if m.Paused == nil {
			m.Paused = map[int]core.ProcessInfo{}
		}
		targets := make([]core.ProcessInfo, 0, len(raw))
		for _, t := range raw {
			if _, paused := m.Paused[t.PID]; paused {
				t.State = core.StateStopped // → SIGCONT (resume)
			} else {
				pinned := t
				pinned.CPUPercent = 0
				m.Paused[t.PID] = pinned // optimistic pin
			}
			targets = append(targets, t)
		}
		return m, runAction(m.PauseAction, targets)
	case "I":
		if m.Cursor < 0 || m.Cursor >= len(m.Findings) {
			return m.adjustOffset(), nil
		}
		f := m.Findings[m.Cursor]
		// Only the three real detectors have ignore lists; synthetic
		// "paused" pins cannot be ignored.
		if f.Detector != "runaway" && f.Detector != "zombie" && f.Detector != "herd" && f.Detector != "memleak" {
			return m.adjustOffset(), nil
		}
		pending := f
		m.IgnorePending = &pending
		m.Mode = ModeConfirmIgnore
		return m, nil
	default:
		// Match against registered actions by Key().
		r := []rune(msg.String())
		if len(r) == 1 {
			for _, a := range m.Actions {
				if a.Key() == r[0] {
					m.PendingAction = a
					if a.Destructive() {
						m.Mode = ModeConfirm
						m.ConfirmOffset = 0
					} else {
						return m, runAction(a, m.selectedTargets())
					}
					break
				}
			}
		}
	}
	return m.adjustOffset(), nil
}

func (m Model) handleConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "enter":
		action := m.PendingAction
		targets := m.selectedTargets()
		m.Mode = ModeRunning
		m.ActionRunning = true
		m.SpinnerFrame = 0
		return m, tea.Batch(runAction(action, targets), spinTick())
	case "n":
		m.Mode = ModeList
		return m, nil
	case "up":
		if m.ConfirmOffset > 0 {
			m.ConfirmOffset--
		}
		return m, nil
	case "down":
		m.ConfirmOffset++
		return m.clampConfirmOffset(), nil
	case "pgup":
		page := confirmPageSize(m)
		m.ConfirmOffset -= page
		return m.clampConfirmOffset(), nil
	case "pgdown":
		page := confirmPageSize(m)
		m.ConfirmOffset += page
		return m.clampConfirmOffset(), nil
	case "home":
		m.ConfirmOffset = 0
		return m, nil
	case "end":
		m.ConfirmOffset = 1 << 30 // clamp below
		return m.clampConfirmOffset(), nil
	}
	return m, nil
}

// handleIgnoreConfirmKey handles input while the ignore-confirm modal is open.
func (m Model) handleIgnoreConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "enter":
		if m.IgnorePending == nil {
			m.Mode = ModeList
			return m, nil
		}
		f := *m.IgnorePending
		if err := config.AppendIgnoreAt(m.IgnorePath, f.Detector, f.Process.Command); err != nil {
			m.LastResults = []core.ActionResult{{
				PID: f.Process.PID, Err: err, Message: "ignore failed: " + err.Error(),
			}}
			m.IgnorePending = nil
			m.Mode = ModeResults
			return m, nil
		}
		m = m.filterIgnored(f.Detector, f.Process.Command)
		m.IgnorePending = nil
		m.Mode = ModeList
		return m.adjustOffset(), nil
	case "n":
		m.IgnorePending = nil
		m.Mode = ModeList
		return m, nil
	}
	return m, nil
}

// confirmPageSize returns the per-page step used by PgUp/PgDn inside the
// confirm modal. Falls back to 1 on degenerate terminals.
func confirmPageSize(m Model) int {
	targets := m.selectedTargets()
	n := len(targets)
	hasZombie := false
	for _, p := range targets {
		if strings.HasPrefix(p.Command, "reap ") {
			hasZombie = true
			break
		}
	}
	v := m.confirmVisibleCount(n, hasZombie)
	if v < 1 {
		return 1
	}
	return v
}

// clampConfirmOffset is a thin wrapper that recomputes the visible window
// before clamping. Used by the scroll-key cases in handleConfirmKey.
func (m Model) clampConfirmOffset() Model {
	targets := m.selectedTargets()
	n := len(targets)
	hasZombie := false
	for _, p := range targets {
		if strings.HasPrefix(p.Command, "reap ") {
			hasZombie = true
			break
		}
	}
	visible := m.confirmVisibleCount(n, hasZombie)
	return m.adjustConfirmOffset(n, visible)
}

func runAction(a core.Action, targets []core.ProcessInfo) tea.Cmd {
	return func() tea.Msg {
		results := a.Execute(context.Background(), targets)
		return ActionMsg{Results: results}
	}
}

// runSample executes the Sample action asynchronously and emits SampleDoneMsg
// with the parsed call stacks.
func runSample(target core.ProcessInfo) tea.Cmd {
	return func() tea.Msg {
		s := actions.NewSample()
		_ = s.Execute(context.Background(), []core.ProcessInfo{target})
		return SampleDoneMsg{PID: target.PID, Stacks: s.Stacks[target.PID]}
	}
}

// runFetchInfo loads the lazy info-modal fields for a single PID off the
// UI goroutine. Always returns a InfoDoneMsg so the modal can transition
// out of the loading state even on permission errors.
func runFetchInfo(pid int) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		return InfoDoneMsg{
			PID:     pid,
			Details: actions.FetchInfo(ctx, pid),
		}
	}
}

// findPIDIndex returns the index of the first finding whose primary process
// matches pid. Returns -1 when not found (caller falls back to a clamp).
func findPIDIndex(findings []core.Finding, pid int) int {
	if pid == 0 {
		return -1
	}
	for i, f := range findings {
		if f.Process.PID == pid {
			return i
		}
	}
	return -1
}

// runRescan re-runs the engine and optionally appends the result to the
// history file. Blocking; intended to be returned as a tea.Cmd.
func runRescan(engine *core.Engine, historyEnabled bool, hcfg history.WriterConfig) tea.Cmd {
	return func() tea.Msg {
		start := time.Now()
		findings, err := engine.Run(context.Background())
		dur := time.Since(start)
		if err == nil && historyEnabled && hcfg.Enabled {
			_ = history.RotateIfNeeded(hcfg.MaxSizeMB, hcfg.MaxRotations)
			if w, werr := history.NewWriter(); werr == nil {
				_ = w.AppendRun(history.RunMeta{
					TS: start, Mode: "doctor", DurationMS: dur.Milliseconds(),
				}, findings)
				_ = w.Close()
			}
		}
		return RescanDoneMsg{Findings: findings, Duration: dur, Err: err}
	}
}
