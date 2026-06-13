package doctor

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/d56de/shrike/internal/actions"
	"github.com/d56de/shrike/internal/core"
	"github.com/d56de/shrike/internal/stats"
	"github.com/d56de/shrike/internal/tui/style"
)

// View implements tea.Model.
func (m Model) View() string {
	t := m.Theme
	// Allow zero-value Theme (defensive for tests / NewModel via older call sites).
	if t.Severity == nil {
		t = style.DefaultTheme()
	}

	// Don't render anything until Bubble Tea has told us the real terminal size.
	// Rendering a too-wide frame would cause the terminal to hard-wrap every
	// line, throwing off Bubble Tea's diff renderer and producing duplicate
	// footer/border artefacts (particularly noticeable with p10k instant-prompt).
	if m.Width < 40 {
		if m.Width == 0 {
			return ""
		}
		return "shrike: terminal too narrow (need ≥ 40 columns)"
	}

	width := m.Width
	// Inner (content) width = outer - 2 for border | chars, minus 2 for left padding.
	innerWidth := width - 4
	if innerWidth < 20 {
		innerWidth = 20
	}

	title := "Shrike doctor " + nonEmpty(m.Version, "dev")
	var body string
	switch m.Mode {
	case ModeConfirm:
		name := "confirm"
		if m.PendingAction != nil {
			name = m.PendingAction.Name()
		}
		title = "Shrike — " + name
		body = renderConfirmBody(t, m, innerWidth)
	case ModeConfirmIgnore:
		title = "Shrike — ignore"
		body = renderIgnoreConfirmBody(t, m, innerWidth)
	case ModeRunning:
		name := "running"
		if m.PendingAction != nil {
			name = m.PendingAction.Name()
		}
		title = "Shrike — " + name
		body = renderRunningBody(t, m, innerWidth)
	case ModeResults:
		title = "Shrike — results"
		body = renderResultsBody(t, m, innerWidth)
	case ModeInfo:
		title = "Shrike — info"
		body = renderInfoBody(t, m, innerWidth)
	case ModeSample:
		title = "Shrike — sample"
		body = renderSampleBody(t, m, innerWidth)
	case ModeHelp:
		title = "Shrike — help"
		body = renderHelpBody(t, innerWidth)
	case ModeStats:
		title = "Shrike — trends"
		body = renderStatsBody(t, m, innerWidth)
	default:
		body = renderListBody(t, m, innerWidth)
	}

	return frame(t, title, body, width)
}

// footerDivider renders the horizontal rule shown above the keyhint footer
// of every screen, styled in the same Frame colour as the border.
func footerDivider(t style.Theme, innerWidth int) string {
	return pad(t.Frame.Render(strings.Repeat("─", innerWidth-4))) + "\n"
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

// shrikeLogo is the ASCII-art logo rendered at the top of the main screen.
const shrikeLogo = `███████╗██╗  ██╗██████╗ ██╗██╗  ██╗███████╗
██╔════╝██║  ██║██╔══██╗██║██║ ██╔╝██╔════╝
███████╗███████║██████╔╝██║█████╔╝ █████╗
╚════██║██╔══██║██╔══██╗██║██╔═██╗ ██╔══╝
███████║██║  ██║██║  ██║██║██║  ██╗███████╗
╚══════╝╚═╝  ╚═╝╚═╝  ╚═╝╚═╝╚═╝  ╚═╝╚══════╝`

// renderListBody returns the inner body (without frame) for the main doctor list.
// innerWidth is the target usable width inside the frame (excluding │ borders).
func renderListBody(t style.Theme, m Model, innerWidth int) string {
	var b strings.Builder

	// Leading blank padding (top inner margin) — two lines so the ASCII logo
	// has breathing room from the frame border.
	b.WriteString(pad("") + "\n")
	b.WriteString(pad("") + "\n")

	// Fancy logo — skip on terminals too narrow to show it cleanly.
	if innerWidth >= 46 {
		for _, line := range strings.Split(shrikeLogo, "\n") {
			b.WriteString(pad(t.Frame.Render(line)) + "\n")
		}
		b.WriteString(pad("") + "\n")
	}

	// Status header: "⏺ N suspicious processes found (X.XXs)"
	durLabel := "—"
	if m.RunDuration > 0 {
		durLabel = fmt.Sprintf("%.2fs", m.RunDuration.Seconds())
	}
	statusTailInner := durLabel
	if m.AutoRefreshInterval > 0 {
		if m.AutoRefreshOn {
			statusTailInner += " · auto: " + shortDuration(m.AutoRefreshInterval)
		} else {
			statusTailInner += " · auto: off"
		}
	}
	statusTail := t.Subtle.Render("(" + statusTailInner + ")")
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
		b.WriteString(pad(t.CheckboxOn.Render("✓")+" "+t.Subtle.Render("No suspicious processes — your Mac looks great")) + "\n")
		b.WriteString(pad(t.Subtle.Render("(Lower the thresholds in config.toml to make shrike more picky.)")) + "\n")
	}

	// Window the list so it fits in the terminal — see Model.visibleCount /
	// Model.adjustOffset. When everything fits, start=0 / end=len: identical
	// to pre-scroll behaviour.
	visible := m.visibleCount()
	start := m.Offset
	if start < 0 {
		start = 0
	}
	end := start + visible
	if end > len(m.Findings) {
		end = len(m.Findings)
	}

	if start > 0 {
		b.WriteString(pad(t.Subtle.Render(fmt.Sprintf("↑ %d more above", start))) + "\n")
		b.WriteString(pad(t.Gutter.Render("│")) + "\n")
	}

	for i := start; i < end; i++ {
		f := m.Findings[i]
		active := m.Cursor == i
		killed := isFindingKilled(f, m.KilledPIDs)

		// Cursor glyph replaces the tree-gutter at the entry row.
		var cursor string
		switch {
		case active:
			cursor = t.Cursor.Render("◆")
		default:
			cursor = t.CursorInactive.Render("◇")
		}

		// Checkbox: ✓ for selected, ✕ when killed (overrides selection),
		// faded ✓ otherwise.
		var box string
		switch {
		case killed:
			box = t.Killed.Render("✕")
		case m.Selected[i]:
			box = t.CheckboxOn.Render("✓")
		default:
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
			cpuPrefix = t.Subtle.Render("Σ")
		}

		bar := renderCPUBarColored(t, cpu, 6, sevName)
		rssLabel := fmt.Sprintf("%d MB", rss/1024/1024)

		// Killed entries: strike through the data half (cmd/PID/CPU/RSS/age)
		// so the row stays in place after a kill — the user can still see
		// what was acted on. Leave cursor + checkbox glyph un-restyled so
		// they remain readable; the ✕ in the box already carries the state.
		data := fmt.Sprintf("%-30s  PID %-6d %s %s%.1f%% CPU · %-7s · %-7s %s",
			cmdLabel, f.Process.PID, bar, cpuPrefix, cpu, rssLabel,
			formatElapsedShort(int64(f.Process.ElapsedTime.Seconds())), sev)
		// Pinned paused rows get a marker; a killed row keeps its strikethrough.
		if m.isPaused(f.Process.PID) && !killed {
			data = data + "  " + t.Accent.Render("⏸ paused")
		}
		if killed {
			data = t.Killed.Render(data)
		}
		row := fmt.Sprintf("%s  %s %s", cursor, box, data)
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

		// Herd expansion — list all members when user pressed [→].
		if f.Group != nil && m.Expanded[i] {
			members := make([]core.ProcessInfo, 0, len(f.Group.Children)+1)
			members = append(members, f.Group.Parent)
			members = append(members, f.Group.Children...)
			for j, child := range members {
				branch := "├─"
				if j == len(members)-1 {
					branch = "└─"
				}
				line := fmt.Sprintf("%s PID %-6d  %.1f%% CPU · %d MB · %s",
					t.Gutter.Render(branch),
					child.PID,
					child.CPUPercent,
					child.RSS/1024/1024,
					formatElapsedShort(int64(child.ElapsedTime.Seconds())))
				b.WriteString(pad(t.Gutter.Render("│")+"   "+t.Subtle.Render(line)) + "\n")
			}
		}

		// Connector between entries (except after the last visible entry).
		if i < end-1 {
			b.WriteString(pad(t.Gutter.Render("│")) + "\n")
		}
	}

	if end < len(m.Findings) {
		b.WriteString(pad(t.Gutter.Render("│")) + "\n")
		b.WriteString(pad(t.Subtle.Render(fmt.Sprintf("↓ %d more below", len(m.Findings)-end))) + "\n")
	}

	b.WriteString(pad("") + "\n")
	b.WriteString(footerDivider(t, innerWidth))
	b.WriteString(pad(t.Frame.Render(fmt.Sprintf("%d selected", len(m.Selected)))) + "\n")
	b.WriteString(pad("") + "\n")
	// Footer: only mention [→] expand if at least one herd finding is present.
	hasHerd := false
	for _, f := range m.Findings {
		if f.Group != nil {
			hasHerd = true
			break
		}
	}
	for _, line := range wrapKeyhint(keyhintSegments(hasHerd, m.AutoRefreshInterval > 0), innerWidth) {
		b.WriteString(pad(t.KeyHint.Render(line)) + "\n")
	}
	return b.String()
}

// keyhintSegments returns the discrete keyhint segments shown in the footer.
// Building them as a slice lets wrapKeyhint flow them across multiple lines
// when the terminal is too narrow for a single-line footer.
func keyhintSegments(hasHerd, hasAutoRefresh bool) []string {
	segs := []string{"[↑/↓] navigate", "[PgUp/PgDn] page", "[space] select"}
	if hasHerd {
		segs = append(segs, "[→] expand")
	}
	segs = append(segs,
		"[i]nfo", "[s]ample", "[k]ill", "[r]enice", "[p]ause", "[I]gnore",
		"[t] trends", "[R] rescan")
	if hasAutoRefresh {
		segs = append(segs, "[a]uto-refresh")
	}
	return append(segs, "[?] help", "[q]uit")
}

// shortDuration formats a duration as the compact form used in the status
// header: "5s", "30s", "2m", "1h". Sub-second durations fall back to "<1s".
func shortDuration(d time.Duration) string {
	switch {
	case d < time.Second:
		return "<1s"
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
}

// wrapKeyhint flows segments into lines, joined by " · ", so each line fits
// within width visual columns. Segments are never split — a segment longer
// than width sits alone on its own line (and overflows).
func wrapKeyhint(segments []string, width int) []string {
	if width < 1 {
		width = 1
	}
	const sep = " · "
	sepW := lipgloss.Width(sep)
	var lines []string
	cur := ""
	curW := 0
	for _, s := range segments {
		sw := lipgloss.Width(s)
		if cur == "" {
			cur = s
			curW = sw
			continue
		}
		if curW+sepW+sw <= width {
			cur += sep + s
			curW += sepW + sw
			continue
		}
		lines = append(lines, cur)
		cur = s
		curW = sw
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return lines
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
//
// Every cell is a "█" so adjacent cells join seamlessly regardless of font:
//   - filled cells use the severity foreground colour
//   - the boundary cell uses a partial-block glyph with severity foreground
//     and the empty colour as background, so its right portion blends into
//     the empty cells next to it
//   - empty cells use a "█" in the empty colour
//
// This eliminates the visual gap that appeared at low percentages when the
// empty portion was rendered with "░" (LIGHT SHADE) — that glyph's dotted
// fill doesn't reach the cell edges in most monospace fonts, so the
// transition `█░` showed a vertical seam.
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
	emptyColor := lipgloss.Color("237")
	emptyStyle := lipgloss.NewStyle().Foreground(emptyColor)

	var s strings.Builder
	s.WriteString(sevStyle.Render(strings.Repeat("█", fullBlocks)))
	remaining := width - fullBlocks
	if remainder > 0 && remaining > 0 {
		s.WriteString(sevStyle.Background(emptyColor).Render(partialBlocks[remainder]))
		remaining--
	}
	s.WriteString(emptyStyle.Render(strings.Repeat("█", remaining)))
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

func renderConfirmBody(t style.Theme, m Model, innerWidth int) string {
	a := m.PendingAction
	if a == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(pad("") + "\n")

	// Header: ⏺ status marker + action question.
	b.WriteString(pad(t.Accent.Render("⏺")+"  "+a.Confirm()) + "\n")
	b.WriteString(pad(t.Gutter.Render("│")) + "\n")

	targets := m.selectedTargets()

	// Pre-pass: detect zombie redirects so chrome budget for the viewport is
	// computed correctly (zombie warning adds two lines below the list).
	hasZombieRedirect := false
	for _, p := range targets {
		if strings.HasPrefix(p.Command, "reap ") {
			hasZombieRedirect = true
			break
		}
	}

	// Window the targets list so a herd-kill of N processes doesn't overflow
	// the terminal. Scroll keys (↑/↓/PgUp/PgDn/Home/End) adjust m.ConfirmOffset.
	visible := m.confirmVisibleCount(len(targets), hasZombieRedirect)
	offset := m.ConfirmOffset
	if visible >= len(targets) {
		offset = 0
	} else if max := len(targets) - visible; offset > max {
		offset = max
	}
	if offset < 0 {
		offset = 0
	}
	end := offset + visible
	if end > len(targets) {
		end = len(targets)
	}
	hiddenAbove := offset
	hiddenBelow := len(targets) - end
	scrollable := hiddenAbove > 0 || hiddenBelow > 0

	// "↑ N more" indicator when scrolled.
	if hiddenAbove > 0 {
		b.WriteString(pad(t.Subtle.Render(fmt.Sprintf("  ↑ %d more above", hiddenAbove))) + "\n")
	}

	// One row per visible target, styled like a main-list entry, with a
	// gutter connector line between rows so it visually reads as a small
	// tree. Zombie redirects are rendered un-truncated so the user always
	// sees that the action will hit a different PID than the one selected.
	for i := offset; i < end; i++ {
		p := targets[i]
		cursor := t.Cursor.Render("◆")
		if strings.HasPrefix(p.Command, "reap ") {
			warn := t.Severity["high"].Render("⚠")
			b.WriteString(pad(fmt.Sprintf("%s  %s %s", cursor, warn, p.Command)) + "\n")
		} else {
			sevName := severityForPID(m.Findings, p.PID)
			bar := renderCPUBarColored(t, p.CPUPercent, 6, sevName)
			line := fmt.Sprintf("%s  %-30s  PID %-6d %s %.1f%% CPU",
				cursor, truncate(p.Command, 30), p.PID, bar, p.CPUPercent)
			b.WriteString(pad(line) + "\n")
		}
		if i < end-1 {
			b.WriteString(pad(t.Gutter.Render("│")) + "\n")
		}
	}

	// "↓ N more" indicator when more targets remain below the viewport.
	if hiddenBelow > 0 {
		b.WriteString(pad(t.Subtle.Render(fmt.Sprintf("  ↓ %d more below", hiddenBelow))) + "\n")
	}

	b.WriteString(pad(t.Gutter.Render("│")) + "\n")

	if hasZombieRedirect {
		b.WriteString(pad(t.Severity["high"].Render(
			"⚠ Parent process will be signalled — may terminate a running GUI app.")) + "\n")
		b.WriteString(pad(t.Subtle.Render(
			"  Zombies themselves consume nothing; safe to leave alone.")) + "\n")
	}

	b.WriteString(pad("") + "\n")
	b.WriteString(footerDivider(t, innerWidth))
	b.WriteString(pad("") + "\n")
	hint := "[y] confirm · [n] cancel"
	if scrollable {
		hint = "[↑/↓] scroll · " + hint
	}
	b.WriteString(pad(t.KeyHint.Render(hint)) + "\n")
	return b.String()
}

// renderIgnoreConfirmBody renders the ignore-from-TUI confirmation modal.
func renderIgnoreConfirmBody(t style.Theme, m Model, innerWidth int) string {
	f := m.IgnorePending
	if f == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(pad("") + "\n")
	q := fmt.Sprintf("Ignore all '%s' processes in the %s detector?",
		f.Process.Command, f.Detector)
	b.WriteString(pad(t.Accent.Render("⏺")+"  "+q) + "\n")
	b.WriteString(pad(t.Gutter.Render("│")) + "\n")
	b.WriteString(pad(t.Subtle.Render("  Saved to ignore.toml — they won't be flagged again.")) + "\n")
	b.WriteString(pad(t.Subtle.Render("  Edit or delete that file by hand to undo.")) + "\n")
	b.WriteString(pad("") + "\n")
	b.WriteString(footerDivider(t, innerWidth))
	b.WriteString(pad("") + "\n")
	b.WriteString(pad(t.KeyHint.Render("[y] confirm · [n] cancel")) + "\n")
	return b.String()
}

// isFindingKilled reports whether the user already signalled the process
// represented by this finding in the current session. For herds, any group
// member match counts (bulk-kill signals every PID). For zombies, the kill
// targets the PPID, so the parent PID is what lands in `killed`.
func isFindingKilled(f core.Finding, killed map[int]bool) bool {
	if len(killed) == 0 {
		return false
	}
	if killed[f.Process.PID] {
		return true
	}
	if f.Detector == "zombie" && killed[f.Process.PPID] {
		return true
	}
	if f.Group != nil {
		if killed[f.Group.Parent.PID] {
			return true
		}
		for _, c := range f.Group.Children {
			if killed[c.PID] {
				return true
			}
		}
	}
	return false
}

// severityForPID looks up the severity label associated with a PID across the
// current findings (including herd children). Falls back to "low" if the PID
// isn't tracked.
func severityForPID(findings []core.Finding, pid int) string {
	for _, f := range findings {
		if f.Process.PID == pid {
			return f.Severity.String()
		}
		if f.Group != nil {
			for _, c := range f.Group.Children {
				if c.PID == pid {
					return f.Severity.String()
				}
			}
		}
	}
	return "low"
}

func renderRunningBody(t style.Theme, m Model, innerWidth int) string {
	var b strings.Builder
	b.WriteString(pad("") + "\n")

	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	spin := t.Frame.Render(frames[m.SpinnerFrame%len(frames)])

	actionName := "action"
	if m.PendingAction != nil {
		actionName = m.PendingAction.Name()
	}
	targets := m.selectedTargets()
	b.WriteString(pad(spin+"  "+t.Subtle.Render(fmt.Sprintf("running %s on %d process(es)…",
		actionName, len(targets)))) + "\n")
	b.WriteString(pad(t.Gutter.Render("│")) + "\n")

	for _, p := range targets {
		sevName := severityForPID(m.Findings, p.PID)
		cursor := t.CursorInactive.Render("◇")
		bar := renderCPUBarColored(t, p.CPUPercent, 6, sevName)
		line := fmt.Sprintf("%s  %-30s  PID %-6d %s %.1f%% CPU",
			cursor, truncate(p.Command, 30), p.PID, bar, p.CPUPercent)
		b.WriteString(pad(line) + "\n")
	}

	b.WriteString(pad("") + "\n")
	b.WriteString(footerDivider(t, innerWidth))
	b.WriteString(pad("") + "\n")
	b.WriteString(pad(t.KeyHint.Render("please wait — kill uses SIGTERM with 3s escalation")) + "\n")
	return b.String()
}

func renderResultsBody(t style.Theme, m Model, innerWidth int) string {
	var b strings.Builder
	b.WriteString(pad("") + "\n")

	ok, failed := 0, 0
	for _, r := range m.LastResults {
		if r.Err != nil {
			failed++
		} else {
			ok++
		}
	}
	summary := fmt.Sprintf("%d succeeded · %d failed", ok, failed)
	b.WriteString(pad(t.Accent.Render("⏺")+"  "+summary) + "\n")
	b.WriteString(pad(t.Gutter.Render("│")) + "\n")

	for _, r := range m.LastResults {
		mark := t.CheckboxOn.Render("✓")
		if r.Err != nil {
			mark = t.Severity["high"].Render("✗")
		}
		b.WriteString(pad(fmt.Sprintf("%s  PID %-6d  %s", mark, r.PID, r.Message)) + "\n")
	}

	b.WriteString(pad("") + "\n")
	b.WriteString(footerDivider(t, innerWidth))
	b.WriteString(pad("") + "\n")
	b.WriteString(pad(t.KeyHint.Render("[any key] back")) + "\n")
	return b.String()
}

func renderInfoBody(t style.Theme, m Model, innerWidth int) string {
	if m.Cursor < 0 || m.Cursor >= len(m.Findings) {
		return ""
	}
	f := m.Findings[m.Cursor]
	p := f.Process

	bar := renderCPUBarColored(t, p.CPUPercent, 6, f.Severity.String())

	rows := [][2]string{
		{"Command:", p.FullPath},
		{"Args:", strings.Join(p.Args, " ")},
		{"User:", p.User},
		{"Parent:", fmt.Sprintf("PID %d", p.PPID)},
		{"Started:", fmt.Sprintf("%s (%s ago)", p.StartedAt.Format("2006-01-02 15:04:05"),
			formatElapsedShort(int64(p.ElapsedTime.Seconds())))},
		{"State:", p.State.String()},
		{"Nice:", fmt.Sprintf("%d", p.Nice)},
		{"CPU:", fmt.Sprintf("%s  %.1f%%", bar, p.CPUPercent)},
		{"RSS:", fmt.Sprintf("%d MB", p.RSS/1024/1024)},
		{"VSZ:", fmt.Sprintf("%d MB", p.VSZ/1024/1024)},
	}

	var b strings.Builder
	b.WriteString(pad("") + "\n")
	for _, r := range rows {
		b.WriteString(pad(formatKV(t, r[0], r[1], 12)) + "\n")
	}

	// Detail section: extra fields loaded lazily on modal open. The static
	// rows above stay visible during the fetch so the panel never blanks.
	b.WriteString(pad("") + "\n")
	b.WriteString(pad(t.Subtle.Render("─ details ──────────────────────────")) + "\n")

	if m.InfoLoading {
		frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		spin := t.Frame.Render(frames[m.SpinnerFrame%len(frames)])
		b.WriteString(pad(spin+" "+t.Subtle.Render("loading process details…")) + "\n")
	} else {
		d := m.InfoDetails
		detailRows := [][2]string{
			{"Cwd:", nonEmpty(d.Cwd, "—")},
			{"Threads:", fmt.Sprintf("%d", d.Threads)},
			{"I/O read:", actions.FormatBytes(d.IOReads)},
			{"I/O write:", actions.FormatBytes(d.IOWrites)},
			{"Ancestry:", actions.FormatAncestors(d.Ancestors)},
		}
		// When the parent finding is a herd, list the helper children so the
		// inspector reads as a tree-summary, not just the single representative.
		if f.Group != nil && len(f.Group.Children) > 0 {
			detailRows = append(detailRows,
				[2]string{"Herd kids:", fmt.Sprintf("%d helpers", len(f.Group.Children))})
		}
		for _, r := range detailRows {
			b.WriteString(pad(formatKV(t, r[0], r[1], 12)) + "\n")
		}
		for _, note := range d.Notes {
			b.WriteString(pad(t.Subtle.Render("note: "+note)) + "\n")
		}
	}

	b.WriteString(pad("") + "\n")
	b.WriteString(footerDivider(t, innerWidth))
	b.WriteString(pad("") + "\n")
	b.WriteString(pad(t.KeyHint.Render("[esc] back")) + "\n")
	return b.String()
}

// formatKV renders a key/value row with the key styled Subtle and padded to
// keyWidth visible cells so the value column aligns across rows regardless
// of key length or ANSI escape codes in the styled output.
func formatKV(t style.Theme, key, value string, keyWidth int) string {
	padding := keyWidth - lipgloss.Width(key)
	if padding < 0 {
		padding = 0
	}
	return t.Subtle.Render(key) + strings.Repeat(" ", padding) + value
}

func renderSampleBody(t style.Theme, m Model, innerWidth int) string {
	if m.Cursor < 0 || m.Cursor >= len(m.Findings) {
		return ""
	}
	var b strings.Builder
	b.WriteString(pad("") + "\n")

	switch {
	case m.Sampling:
		frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		spin := t.Frame.Render(frames[m.SpinnerFrame%len(frames)])
		b.WriteString(pad(spin+" "+t.Subtle.Render("running sample… (takes ~5 seconds)")) + "\n")
	case len(m.SampleStacks) == 0:
		b.WriteString(pad(t.Subtle.Render("no samples parsed — process may have exited or sample(1) failed")) + "\n")
	default:
		b.WriteString(pad("Hottest call stacks:") + "\n")
		b.WriteString(pad("") + "\n")
		sevName := m.Findings[m.Cursor].Severity.String()
		for i, s := range m.SampleStacks {
			bar := renderCPUBarColored(t, s.Percent, 6, sevName)
			b.WriteString(pad(fmt.Sprintf("%d. %s  %4.1f%%  %s", i+1, bar, s.Percent, s.Top)) + "\n")
		}
	}

	b.WriteString(pad("") + "\n")
	b.WriteString(footerDivider(t, innerWidth))
	b.WriteString(pad("") + "\n")
	b.WriteString(pad(t.KeyHint.Render("[esc] back")) + "\n")
	return b.String()
}

func renderHelpBody(t style.Theme, innerWidth int) string {
	items := [][2]string{
		{"↑/↓", "navigate"},
		{"PgUp/PgDn", "page up/down"},
		{"Home/End", "first / last finding"},
		{"Space", "select / deselect"},
		{"→/←", "expand/collapse herd"},
		{"i", "info modal"},
		{"s", "sample 5s"},
		{"k", "kill (TERM → KILL)"},
		{"K", "kill immediately (SIGKILL)"},
		{"r", "renice +10"},
		{"p", "pause / resume (toggle SIGSTOP/SIGCONT)"},
		{"I", "ignore process (saved to ignore.toml)"},
		{"R", "rescan (re-run detectors)"},
		{"t", "activity trends (heatmap)"},
		{"a", "toggle auto-refresh (configure interval in config.toml)"},
		{"?", "this help"},
		{"q / Esc", "quit / close modal"},
	}
	var b strings.Builder
	b.WriteString(pad("") + "\n")
	for _, it := range items {
		b.WriteString(pad(formatKV(t, it[0], it[1], 12)) + "\n")
	}
	b.WriteString(pad("") + "\n")
	b.WriteString(footerDivider(t, innerWidth))
	b.WriteString(pad("") + "\n")
	b.WriteString(pad(t.KeyHint.Render("[esc] back")) + "\n")
	return b.String()
}

// renderStatsBody renders the activity heatmap (reusing internal/stats) sized
// to fit the frame. Read live — the trends view triggers no render ticks, so
// View() runs only on real events while it is open.
func renderStatsBody(t style.Theme, m Model, innerWidth int) string {
	weeks := stats.WeeksForWidth(innerWidth - 2) // -2 for pad()'s left margin
	to := time.Now()
	from := to.AddDate(0, 0, -7*weeks+1)

	var b strings.Builder
	b.WriteString(pad("") + "\n")
	days, summary, err := stats.Aggregate(from, to)
	if err != nil {
		b.WriteString(pad(t.Subtle.Render("trends unavailable: "+err.Error())) + "\n")
	} else if summary.ActiveDays == 0 {
		b.WriteString(pad(t.Subtle.Render("(no activity in window)")) + "\n")
	} else {
		heatmap := stats.Render(days, summary, stats.RenderOptions{Metric: "scans"})
		for _, line := range strings.Split(strings.TrimRight(heatmap, "\n"), "\n") {
			b.WriteString(pad(line) + "\n")
		}
	}
	b.WriteString(pad("") + "\n")
	b.WriteString(footerDivider(t, innerWidth))
	b.WriteString(pad(t.KeyHint.Render("[esc] back")) + "\n")
	return b.String()
}
