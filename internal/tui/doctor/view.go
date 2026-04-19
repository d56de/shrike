package doctor

import (
	"fmt"
	"strings"

	"github.com/d56de/shrike/internal/tui/style"
)

// View implements tea.Model.
func (m Model) View() string {
	t := style.DefaultTheme()
	switch m.Mode {
	case ModeConfirm:
		return renderConfirm(t, m)
	case ModeResults:
		return renderResults(t, m)
	case ModeInfo:
		return renderInfo(t, m)
	case ModeSample:
		return renderSample(t, m)
	case ModeHelp:
		return renderHelp(t)
	}
	return renderList(t, m)
}

func renderList(t style.Theme, m Model) string {
	var b strings.Builder
	b.WriteString(t.Title.Render("shrike doctor") + "   " + t.Subtle.Render("macOS · suspicious processes") + "\n\n")

	if len(m.Findings) == 0 {
		b.WriteString(t.Subtle.Render("No suspicious processes — your Mac looks great.\n"))
	}

	for i, f := range m.Findings {
		cursor := "  "
		if m.Cursor == i {
			cursor = t.Cursor.Render("▶ ")
		}
		box := t.CheckboxOff.Render("☐")
		if m.Selected[i] {
			box = t.CheckboxOn.Render("☑")
		}

		sevName := f.Severity.String()
		sevLabel := strings.ToUpper(sevName[:1]) + sevName[1:]
		sev := t.Severity[sevName].Render(sevLabel)
		emoji := emojiForDetector(f.Detector)

		line1 := fmt.Sprintf("%s%s %s %-30s  PID %-6d %.1f%% CPU · %-10s %s",
			cursor, box, emoji, f.Process.Command, f.Process.PID,
			f.Process.CPUPercent, formatElapsedShort(int64(f.Process.ElapsedTime.Seconds())), sev)
		b.WriteString(line1 + "\n")
		line2 := "       " + t.Subtle.Render(truncatePath(f.Process.FullPath, 78))
		b.WriteString(line2 + "\n")
		if m.Cursor == i {
			b.WriteString("       " + t.Subtle.Render(f.Reason) + "\n")
		}
		b.WriteString("\n")
	}

	b.WriteString(strings.Repeat("─", 78) + "\n")
	b.WriteString(fmt.Sprintf("%d selected\n\n", len(m.Selected)))
	b.WriteString(t.KeyHint.Render("[space] select · [i]nfo · [s]ample · [k]ill · [r]enice · [?] help · [q]uit"))
	return t.Border.Render(b.String())
}

func emojiForDetector(name string) string {
	switch name {
	case "runaway":
		return "🔥"
	case "zombie":
		return "🧟"
	case "herd":
		return "👥"
	default:
		return "•"
	}
}

func truncatePath(p string, maxLen int) string {
	if len(p) <= maxLen {
		return p
	}
	return "…" + p[len(p)-maxLen+1:]
}

func formatElapsedShort(sec int64) string {
	mins := sec / 60
	if mins < 60 {
		return fmt.Sprintf("%dm", mins)
	}
	hours := mins / 60
	if hours < 24 {
		return fmt.Sprintf("%dh %dm", hours, mins%60)
	}
	return fmt.Sprintf("%dd %dh", hours/24, hours%24)
}

func renderConfirm(t style.Theme, m Model) string {
	a := m.PendingAction
	if a == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(t.Title.Render(fmt.Sprintf("Confirm %s", a.Name())) + "\n\n")
	b.WriteString(a.Confirm() + "\n\n")
	for _, p := range m.selectedTargets() {
		b.WriteString(fmt.Sprintf("  %s  PID %d  %.1f%% CPU\n", p.Command, p.PID, p.CPUPercent))
	}
	b.WriteString("\n")
	b.WriteString(t.KeyHint.Render("[y] confirm · [n] cancel"))
	return t.Border.Render(b.String())
}

func renderResults(t style.Theme, m Model) string {
	var b strings.Builder
	b.WriteString(t.Title.Render("Results") + "\n\n")
	for _, r := range m.LastResults {
		mark := "✓"
		if r.Err != nil {
			mark = "✗"
		}
		b.WriteString(fmt.Sprintf("  %s  PID %-6d  %s\n", mark, r.PID, r.Message))
	}
	b.WriteString("\n")
	b.WriteString(t.KeyHint.Render("[any key] back"))
	return t.Border.Render(b.String())
}

func renderInfo(t style.Theme, m Model) string {
	if m.Cursor < 0 || m.Cursor >= len(m.Findings) {
		return ""
	}
	p := m.Findings[m.Cursor].Process
	var b strings.Builder
	b.WriteString(t.Title.Render(fmt.Sprintf("Info: %s (PID %d)", p.Command, p.PID)) + "\n\n")
	b.WriteString(fmt.Sprintf("Command:  %s\n", p.FullPath))
	b.WriteString(fmt.Sprintf("Args:     %s\n", strings.Join(p.Args, " ")))
	b.WriteString(fmt.Sprintf("User:     %s\n", p.User))
	b.WriteString(fmt.Sprintf("Parent:   PID %d\n", p.PPID))
	b.WriteString(fmt.Sprintf("Started:  %s (%s ago)\n", p.StartedAt.Format("2006-01-02 15:04:05"),
		formatElapsedShort(int64(p.ElapsedTime.Seconds()))))
	b.WriteString(fmt.Sprintf("State:    %s\n", p.State))
	b.WriteString(fmt.Sprintf("Nice:     %d\n", p.Nice))
	b.WriteString(fmt.Sprintf("CPU:      %.1f%%\n", p.CPUPercent))
	b.WriteString(fmt.Sprintf("RSS:      %d MB\n", p.RSS/1024/1024))
	b.WriteString(fmt.Sprintf("VSZ:      %d MB\n", p.VSZ/1024/1024))
	b.WriteString("\n" + t.KeyHint.Render("[esc] back"))
	return t.Border.Render(b.String())
}

func renderSample(t style.Theme, m Model) string {
	var b strings.Builder
	if m.Cursor < 0 || m.Cursor >= len(m.Findings) {
		return ""
	}
	p := m.Findings[m.Cursor].Process
	b.WriteString(t.Title.Render(fmt.Sprintf("Sample: %s (PID %d) · 5s", p.Command, p.PID)) + "\n\n")

	switch {
	case m.Sampling:
		b.WriteString(t.Subtle.Render("running sample… (takes ~5 seconds)\n"))
	case len(m.SampleStacks) == 0:
		b.WriteString(t.Subtle.Render("no samples parsed — process may have exited or sample(1) failed\n"))
	default:
		b.WriteString("Hottest call stacks:\n\n")
		for i, s := range m.SampleStacks {
			b.WriteString(fmt.Sprintf("  %d. [%4.1f%%]  %s\n", i+1, s.Percent, s.Top))
		}
	}

	b.WriteString("\n" + t.KeyHint.Render("[esc] back"))
	return t.Border.Render(b.String())
}

func renderHelp(t style.Theme) string {
	items := [][2]string{
		{"↑/↓ j/k", "navigate"},
		{"Space", "select / deselect"},
		{"→/←", "expand/collapse herd"},
		{"i", "info modal"},
		{"s", "sample 5s"},
		{"k", "kill (TERM → KILL)"},
		{"K", "kill immediately (SIGKILL)"},
		{"r", "renice +10"},
		{"?", "this help"},
		{"q / Esc", "quit / close modal"},
	}
	var b strings.Builder
	b.WriteString(t.Title.Render("Help") + "\n\n")
	for _, it := range items {
		b.WriteString(fmt.Sprintf("  %-12s  %s\n", t.KeyHint.Render(it[0]), it[1]))
	}
	b.WriteString("\n" + t.KeyHint.Render("[esc] back"))
	return t.Border.Render(b.String())
}
