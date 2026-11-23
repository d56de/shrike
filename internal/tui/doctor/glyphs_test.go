package doctor

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/d56de/shrike/internal/core"
)

func TestListGlyphsAndNoGutter(t *testing.T) {
	m := listModel(100)
	m.Findings = append(m.Findings, core.Finding{Detector: "runaway", Process: core.ProcessInfo{PID: 7, Command: "other"}})
	m.Selected = map[int]bool{1: true}
	out := ansi.Strip(m.View())
	if !strings.Contains(out, "➤  ○ node-some-long-command") {
		t.Errorf("active unselected row should read ➤ ○:\n%s", out)
	}
	if !strings.Contains(out, "   ● other") {
		t.Errorf("inactive selected row should read ●:\n%s", out)
	}
	for _, line := range strings.Split(out, "\n") {
		inner := []rune(strings.TrimSpace(line))
		if len(inner) > 2 && strings.ContainsAny(string(inner[1:len(inner)-1]), "│├└◆◇") {
			t.Errorf("gutter or old glyph left in: %q", line)
		}
	}
}
