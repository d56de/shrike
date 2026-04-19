package doctor

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/d56de/shrike/internal/actions"
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

// RescanDoneMsg carries the result of an in-TUI rescan.
type RescanDoneMsg struct {
	Findings []core.Finding
	Duration time.Duration
	Err      error
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd { return nil }

// Update implements tea.Model: dispatches based on mode.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.Width, m.Height = msg.Width, msg.Height
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	case ActionMsg:
		m.LastResults = msg.Results
		m.Mode = ModeResults
		return m, nil
	case SampleDoneMsg:
		m.Sampling = false
		m.SamplePID = msg.PID
		m.SampleStacks = msg.Stacks
		return m, nil
	case RescanDoneMsg:
		m.Rescanning = false
		if msg.Err == nil {
			m.Findings = msg.Findings
			m.RunDuration = msg.Duration
			m.Selected = map[int]bool{}
			m.Expanded = map[int]bool{}
			m.Cursor = 0
		}
		return m, nil
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
		return m, nil
	}

	switch m.Mode {
	case ModeList:
		return m.handleListKey(msg)
	case ModeConfirm:
		return m.handleConfirmKey(msg)
	case ModeResults, ModeInfo, ModeSample, ModeHelp:
		// Any key returns to list.
		m.Mode = ModeList
		return m, nil
	}
	return m, nil
}

func (m Model) handleListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.Cursor > 0 {
			m.Cursor--
		}
	case "down", "j":
		if m.Cursor < len(m.Findings)-1 {
			m.Cursor++
		}
	case " ":
		if m.Selected[m.Cursor] {
			delete(m.Selected, m.Cursor)
		} else {
			m.Selected[m.Cursor] = true
		}
	case "right", "l":
		if m.Cursor < len(m.Findings) && m.Findings[m.Cursor].Group != nil {
			m.Expanded[m.Cursor] = true
		}
	case "left", "h":
		delete(m.Expanded, m.Cursor)
	case "R":
		if m.Engine != nil && !m.Rescanning {
			m.Rescanning = true
			return m, runRescan(m.Engine, m.HistoryEnabled, m.HistoryConfig)
		}
	case "i":
		m.Mode = ModeInfo
	case "s":
		if m.Cursor >= 0 && m.Cursor < len(m.Findings) {
			target := m.Findings[m.Cursor].Process
			m.Mode = ModeSample
			m.Sampling = true
			m.SamplePID = target.PID
			m.SampleStacks = nil
			return m, runSample(target)
		}
	default:
		// Match against registered actions by Key().
		r := []rune(msg.String())
		if len(r) == 1 {
			for _, a := range m.Actions {
				if a.Key() == r[0] {
					m.PendingAction = a
					if a.Destructive() {
						m.Mode = ModeConfirm
					} else {
						return m, runAction(a, m.selectedTargets())
					}
					break
				}
			}
		}
	}
	return m, nil
}

func (m Model) handleConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "enter":
		action := m.PendingAction
		targets := m.selectedTargets()
		m.Mode = ModeList
		return m, runAction(action, targets)
	case "n":
		m.Mode = ModeList
		return m, nil
	}
	return m, nil
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
