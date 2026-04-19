package doctor

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/d56de/shrike/internal/core"
)

// ActionMsg is emitted when an Action.Execute returns, carrying results.
type ActionMsg struct {
	Results []core.ActionResult
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
	case "i":
		m.Mode = ModeInfo
	case "s":
		m.Mode = ModeSample
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
