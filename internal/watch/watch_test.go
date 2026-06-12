package watch

import (
	"strings"
	"testing"

	"github.com/d56de/shrike/internal/core"
)

func find(det string, pid int, cmd string, sev core.Severity) core.Finding {
	return core.Finding{
		Detector: det,
		Severity: sev,
		Reason:   "reason",
		Process:  core.ProcessInfo{PID: pid, Command: cmd},
	}
}

func TestDecide_NewFindingAboveLevelNotifies(t *testing.T) {
	w := NewWatcher(core.SeverityHigh)
	got := w.Decide([]core.Finding{find("runaway", 1, "node", core.SeverityHigh)})
	if len(got) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(got))
	}
}

func TestDecide_BelowLevelIgnored(t *testing.T) {
	w := NewWatcher(core.SeverityHigh)
	got := w.Decide([]core.Finding{find("runaway", 1, "node", core.SeverityMedium)})
	if len(got) != 0 {
		t.Fatalf("expected no notification below level, got %d", len(got))
	}
}

func TestDecide_PersistingFindingSilentAfterFirst(t *testing.T) {
	w := NewWatcher(core.SeverityHigh)
	f := find("runaway", 1, "node", core.SeverityHigh)
	if got := w.Decide([]core.Finding{f}); len(got) != 1 {
		t.Fatalf("first scan should notify, got %d", len(got))
	}
	if got := w.Decide([]core.Finding{f}); len(got) != 0 {
		t.Fatalf("persisting finding should be silent, got %d", len(got))
	}
}

func TestDecide_EscalationNotifies(t *testing.T) {
	w := NewWatcher(core.SeverityHigh)
	w.Decide([]core.Finding{find("memleak", 1, "vm", core.SeverityHigh)})
	got := w.Decide([]core.Finding{find("memleak", 1, "vm", core.SeverityCritical)})
	if len(got) != 1 {
		t.Fatalf("escalation should notify, got %d", len(got))
	}
}

func TestDecide_DeEscalationDoesNotNotify(t *testing.T) {
	w := NewWatcher(core.SeverityHigh)
	w.Decide([]core.Finding{find("memleak", 1, "vm", core.SeverityCritical)})
	if got := w.Decide([]core.Finding{find("memleak", 1, "vm", core.SeverityHigh)}); len(got) != 0 {
		t.Fatalf("de-escalation (still >= level) should be silent, got %d", len(got))
	}
}

func TestDecide_DisappearThenReturnNotifiesAgain(t *testing.T) {
	w := NewWatcher(core.SeverityHigh)
	f := find("runaway", 1, "node", core.SeverityHigh)
	w.Decide([]core.Finding{f})
	w.Decide(nil) // gone this scan
	got := w.Decide([]core.Finding{f})
	if len(got) != 1 {
		t.Fatalf("returning finding should notify again, got %d", len(got))
	}
}

func TestDecide_MultipleFreshSummarized(t *testing.T) {
	w := NewWatcher(core.SeverityHigh)
	got := w.Decide([]core.Finding{
		find("runaway", 1, "node", core.SeverityHigh),
		find("memleak", 2, "vm", core.SeverityHigh),
	})
	if len(got) != 1 {
		t.Fatalf("expected a single summary notification, got %d", len(got))
	}
	if got[0].Title != "Shrike: 2 new issues" {
		t.Errorf("title = %q, want 'Shrike: 2 new issues'", got[0].Title)
	}
}

func TestDecide_SingleFreshDetailed(t *testing.T) {
	w := NewWatcher(core.SeverityHigh)
	got := w.Decide([]core.Finding{find("runaway", 7, "node", core.SeverityHigh)})
	if len(got) != 1 {
		t.Fatal("expected 1 notification")
	}
	if got[0].Title != "Shrike: High runaway" {
		t.Errorf("title = %q, want 'Shrike: High runaway'", got[0].Title)
	}
	if !strings.Contains(got[0].Message, "🔥") || !strings.Contains(got[0].Message, "node") {
		t.Errorf("message = %q, want emoji + command", got[0].Message)
	}
}
