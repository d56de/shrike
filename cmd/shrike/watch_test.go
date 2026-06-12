package main

import (
	"strings"
	"testing"

	"github.com/d56de/shrike/internal/core"
)

func TestParseLevel(t *testing.T) {
	if l, err := parseLevel("critical"); err != nil || l != core.SeverityCritical {
		t.Errorf("critical: got (%v, %v)", l, err)
	}
	if l, err := parseLevel("HIGH"); err != nil || l != core.SeverityHigh {
		t.Errorf("HIGH: got (%v, %v)", l, err)
	}
	if _, err := parseLevel("bogus"); err == nil {
		t.Error("expected error for bogus level")
	}
}

func TestWatchLine(t *testing.T) {
	if got := watchLine(nil); !strings.Contains(got, "clean") {
		t.Errorf("empty scan line = %q, want 'clean'", got)
	}
	line := watchLine([]core.Finding{
		{Detector: "runaway", Process: core.ProcessInfo{Command: "node"}},
	})
	if !strings.Contains(line, "🔥") || !strings.Contains(line, "node") {
		t.Errorf("line = %q, want emoji + command", line)
	}
}
