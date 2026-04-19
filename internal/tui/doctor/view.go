package doctor

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/d56de/shrike/internal/tui/style"
)

// View implements tea.Model.
func (m Model) View() string {
	t := m.Theme
	// Allow zero-value Theme (defensive for tests / NewModel via older call sites).
	if t.Severity == nil {
		t = style.DefaultTheme()
	}

	// Target width: use terminal width if known, otherwise a sensible default.
	width := m.Width
	if width < 50 {
		width = 96
	}
	// Inner (content) width = outer - 2 for border | chars, minus 2 for left padding.
	innerWidth := width - 4
	if innerWidth < 20 {
		innerWidth = 20
	}

	title := "Shrike doctor " + nonEmpty(m.Version, "dev")
	var body string
	switch m.Mode {
	case ModeConfirm:
		title = "Shrike — confirm"
		body = renderConfirmBody(t, m)
	case ModeResults:
		title = "Shrike — results"
		body = renderResultsBody(t, m)
	case ModeInfo:
		title = "Shrike — info"
		body = renderInfoBody(t, m)
	case ModeSample:
		title = "Shrike — sample"
		body = renderSampleBody(t, m)
	case ModeHelp:
		title = "Shrike — help"
		body = renderHelpBody(t)
	default:
		body = renderListBody(t, m, innerWidth)
	}

	return frame(t, title, body, width)
}

// frame wraps content in a rounded border with an embedded title in the top edge.
// Border glyphs use t.Frame; the title text uses t.Title.
func frame(t style.Theme, title, body string, width int) string {
	innerWidth := width - 2 // - 2 for the two │ border chars
	titleText := t.Title.Render(title)
	// "── <title> ──" with the dashes in Frame colour and title in Title colour.
	titleContent := t.Frame.Render("── ") + titleText + t.Frame.Render(" ──")
	titleW := lipgloss.Width(titleContent)
	topFill := innerWidth - titleW
	if topFill < 0 {
		topFill = 0
	}
	top := t.Frame.Render("╭") + titleContent + t.Frame.Render(strings.Repeat("─", topFill)+"╮")

	var out strings.Builder
	out.WriteString(top + "\n")
	side := t.Frame.Render("│")
	for _, line := range strings.Split(body, "\n") {
		w := lipgloss.Width(line)
		pad := innerWidth - w
		if pad < 0 {
			pad = 0
		}
		out.WriteString(side + line + strings.Repeat(" ", pad) + side + "\n")
	}
	out.WriteString(t.Frame.Render("╰" + strings.Repeat("─", innerWidth) + "╯"))
	return out.String()
}

func nonEmpty(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

// renderListBody returns the inner body (without frame) for the main doctor list.
// innerWidth is the target usable width inside the frame (excluding │ borders).
func renderListBody(t style.Theme, m Model, innerWidth int) string {
	var b strings.Builder

	// Leading blank padding (top inner margin).
	b.WriteString(pad("") + "\n")

	// Status header: "⏺ N suspicious processes found (X.XXs)"
	durLabel := "—"
	if m.RunDuration > 0 {
		durLabel = fmt.Sprintf("%.2fs", m.RunDuration.Seconds())
	}
	statusTail := t.Subtle.Render("(" + durLabel + ")")
	if m.Rescanning {
		frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		spin := t.Frame.Render(frames[m.SpinnerFrame%len(frames)])
		statusTail = spin + " " + t.Subtle.Render("rescanning…")
	}
	header := fmt.Sprintf("%s %d suspicious process%s found %s",
		t.Accent.Render("⏺"),
		len(m.Findings), plural(len(m.Findings)),
		statusTail)
	b.WriteString(pad(header) + "\n")

	// Connector line from ⏺ down to first cursor.
	b.WriteString(pad(t.Gutter.Render("│")) + "\n")

	pathMax := innerWidth - 8
	if pathMax < 20 {
		pathMax = 20
	}

	if len(m.Findings) == 0 {
		b.WriteString(pad(t.Subtle.Render("No suspicious processes — your Mac looks great 🎉")) + "\n")
		b.WriteString(pad(t.Subtle.Render("(Lower the thresholds in config.toml to make shrike more picky.)")) + "\n")
	}

	for i, f := range m.Findings {
		active := m.Cursor == i

		// Cursor glyph replaces the tree-gutter at the entry row.
		var cursor string
		if active {
			cursor = t.Cursor.Render("◆")
		} else {
			cursor = t.CursorInactive.Render("◇")
		}

		// Checkbox: always ✓; green when selected, grey when not.
		var box string
		if m.Selected[i] {
			box = t.CheckboxOn.Render("✓")
		} else {
			box = t.CheckboxOff.Render("✓")
		}

		sevName := f.Severity.String()
		sevLabel := strings.ToUpper(sevName[:1]) + sevName[1:]
		sev := t.Subtle.Render(sevLabel)

		// For herds, show aggregate CPU/RSS + "×N" count and a Σ prefix on
		// the CPU % so it's visually obvious the number is a sum across
		// multiple processes (otherwise a row like "PID 75006  304.3% CPU"
		// misleadingly reads as one process pegging 3 cores).
		cpu := f.Process.CPUPercent
		rss := f.Process.RSS
		cmdLabel := truncate(f.Process.Command, 30)
		cpuPrefix := " "
		if f.Group != nil {
			cpu = f.Group.TotalCPU
			rss = f.Group.TotalRSS
			cmdLabel = truncate(f.Process.Command, 26) + fmt.Sprintf(" ×%d", len(f.Group.Children)+1)
			cpuPrefix = "Σ"
		}

		bar := renderCPUBarColored(t, cpu, 6, sevName)
		rssLabel := fmt.Sprintf("%d MB", rss/1024/1024)

		// Row: "◆  ✓ Command            PID ###  ███░░░  49.0% CPU · 154 MB · 7d 1h  High"
		//      (Σ replaces the leading space when the CPU is an aggregate.)
		row := fmt.Sprintf("%s  %s %-30s  PID %-6d %s %s%.1f%% CPU · %-7s · %-7s %s",
			cursor, box, cmdLabel,
			f.Process.PID, bar, cpuPrefix, cpu, rssLabel,
			formatElapsedShort(int64(f.Process.ElapsedTime.Seconds())), sev)
		b.WriteString(pad(row) + "\n")

		// Path line always shown (truncated for long paths).
		b.WriteString(pad(t.Gutter.Render("│")+"   "+t.Subtle.Render(truncatePath(f.Process.FullPath, pathMax))) + "\n")

		// Reason line: only when it adds info beyond the top row.
		// - Runaway: "49% CPU for 7d" — already in top row. Skip.
		// - Herd:    "11 X processes using …" — already in top row. Skip.
		// - Zombie:  "kill parent PID N" — actionable, keep.
		if active && f.Detector == "zombie" && f.Reason != "" {
			b.WriteString(pad(t.Gutter.Render("│")+"   "+t.Subtle.Render(f.Reason)) + "\n")
		}

		// Connector between entries (except after the last).
		if i < len(m.Findings)-1 {
			b.WriteString(pad(t.Gutter.Render("│")) + "\n")
		}
	}

	b.WriteString(pad("") + "\n")
	b.WriteString(pad(t.Frame.Render(strings.Repeat("─", innerWidth-4))) + "\n")
	b.WriteString(pad(t.Frame.Render(fmt.Sprintf("%d selected", len(m.Selected)))) + "\n")
	b.WriteString(pad("") + "\n")
	b.WriteString(pad(t.KeyHint.Render("[space] select · [i]nfo · [s]ample · [k]ill · [r]enice · [R] rescan · [?] help · [q]uit")) + "\n")
	return b.String()
}

// pad returns the line with a 2-space left padding matching the frame's inner edge.
func pad(s string) string {
	return "  " + s
}

// plural returns "es" for zero/many, "" for singular.
func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "es"
}

// partialBlocks maps N eighths-of-a-block to the corresponding Unicode glyph.
// Index 0 is a space (no fill), index 8 is a full "█".
var partialBlocks = []string{" ", "▏", "▎", "▍", "▌", "▋", "▊", "▉"}

// renderCPUBarColored draws a width-char bar tinted with the severity colour.
// Uses Unicode partial blocks for sub-cell accuracy so that e.g. 48.8% of a
// 6-cell bar reads as "██▉░░░" (≈48%) instead of "██░░░░" (≈33%).
func renderCPUBarColored(t style.Theme, pct float64, width int, severity string) string {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}

	// Total eighths to fill across the whole bar.
	totalEighths := int(pct / 100.0 * float64(width*8))
	if totalEighths > width*8 {
		totalEighths = width * 8
	}
	fullBlocks := totalEighths / 8
	remainder := totalEighths % 8

	sevStyle, ok := t.Severity[severity]
	if !ok {
		sevStyle = lipgloss.NewStyle()
	}
	// Drop bold so the bar reads evenly even for "critical".
	sevStyle = sevStyle.Bold(false)
	faint := sevStyle.Faint(true)

	var s strings.Builder
	s.WriteString(sevStyle.Render(strings.Repeat("█", fullBlocks)))
	remaining := width - fullBlocks
	if remainder > 0 && remaining > 0 {
		s.WriteString(sevStyle.Render(partialBlocks[remainder]))
		remaining--
	}
	s.WriteString(faint.Render(strings.Repeat("░", remaining)))
	return s.String()
}

// truncate clamps s to maxLen characters, appending an ellipsis.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-1] + "…"
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

func renderConfirmBody(t style.Theme, m Model) string {
	a := m.PendingAction
	if a == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(pad("") + "\n")
	b.WriteString(pad(a.Confirm()) + "\n")
	b.WriteString(pad("") + "\n")
	for _, p := range m.selectedTargets() {
		b.WriteString(pad(fmt.Sprintf("%s  PID %d  %.1f%% CPU", p.Command, p.PID, p.CPUPercent)) + "\n")
	}
	b.WriteString(pad("") + "\n")
	b.WriteString(pad(t.KeyHint.Render("[y] confirm · [n] cancel")) + "\n")
	return b.String()
}

func renderResultsBody(t style.Theme, m Model) string {
	var b strings.Builder
	b.WriteString(pad("") + "\n")
	for _, r := range m.LastResults {
		mark := "✓"
		if r.Err != nil {
			mark = "✗"
		}
		b.WriteString(pad(fmt.Sprintf("%s  PID %-6d  %s", mark, r.PID, r.Message)) + "\n")
	}
	b.WriteString(pad("") + "\n")
	b.WriteString(pad(t.KeyHint.Render("[any key] back")) + "\n")
	return b.String()
}

func renderInfoBody(t style.Theme, m Model) string {
	if m.Cursor < 0 || m.Cursor >= len(m.Findings) {
		return ""
	}
	p := m.Findings[m.Cursor].Process
	var b strings.Builder
	b.WriteString(pad("") + "\n")
	b.WriteString(pad(fmt.Sprintf("Command:  %s", p.FullPath)) + "\n")
	b.WriteString(pad(fmt.Sprintf("Args:     %s", strings.Join(p.Args, " "))) + "\n")
	b.WriteString(pad(fmt.Sprintf("User:     %s", p.User)) + "\n")
	b.WriteString(pad(fmt.Sprintf("Parent:   PID %d", p.PPID)) + "\n")
	b.WriteString(pad(fmt.Sprintf("Started:  %s (%s ago)", p.StartedAt.Format("2006-01-02 15:04:05"),
		formatElapsedShort(int64(p.ElapsedTime.Seconds())))) + "\n")
	b.WriteString(pad(fmt.Sprintf("State:    %s", p.State)) + "\n")
	b.WriteString(pad(fmt.Sprintf("Nice:     %d", p.Nice)) + "\n")
	b.WriteString(pad(fmt.Sprintf("CPU:      %.1f%%", p.CPUPercent)) + "\n")
	b.WriteString(pad(fmt.Sprintf("RSS:      %d MB", p.RSS/1024/1024)) + "\n")
	b.WriteString(pad(fmt.Sprintf("VSZ:      %d MB", p.VSZ/1024/1024)) + "\n")
	b.WriteString(pad("") + "\n")
	b.WriteString(pad(t.KeyHint.Render("[esc] back")) + "\n")
	return b.String()
}

func renderSampleBody(t style.Theme, m Model) string {
	if m.Cursor < 0 || m.Cursor >= len(m.Findings) {
		return ""
	}
	var b strings.Builder
	b.WriteString(pad("") + "\n")

	switch {
	case m.Sampling:
		b.WriteString(pad(t.Subtle.Render("running sample… (takes ~5 seconds)")) + "\n")
	case len(m.SampleStacks) == 0:
		b.WriteString(pad(t.Subtle.Render("no samples parsed — process may have exited or sample(1) failed")) + "\n")
	default:
		b.WriteString(pad("Hottest call stacks:") + "\n")
		b.WriteString(pad("") + "\n")
		for i, s := range m.SampleStacks {
			b.WriteString(pad(fmt.Sprintf("%d. [%4.1f%%]  %s", i+1, s.Percent, s.Top)) + "\n")
		}
	}

	b.WriteString(pad("") + "\n")
	b.WriteString(pad(t.KeyHint.Render("[esc] back")) + "\n")
	return b.String()
}

func renderHelpBody(t style.Theme) string {
	items := [][2]string{
		{"↑/↓ j/k", "navigate"},
		{"Space", "select / deselect"},
		{"→/←", "expand/collapse herd"},
		{"i", "info modal"},
		{"s", "sample 5s"},
		{"k", "kill (TERM → KILL)"},
		{"K", "kill immediately (SIGKILL)"},
		{"r", "renice +10"},
		{"R", "rescan (re-run detectors)"},
		{"?", "this help"},
		{"q / Esc", "quit / close modal"},
	}
	var b strings.Builder
	b.WriteString(pad("") + "\n")
	for _, it := range items {
		b.WriteString(pad(fmt.Sprintf("%-12s  %s", t.KeyHint.Render(it[0]), it[1])) + "\n")
	}
	b.WriteString(pad("") + "\n")
	b.WriteString(pad(t.KeyHint.Render("[esc] back")) + "\n")
	return b.String()
}
