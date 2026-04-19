package actions

import (
	"context"
	"testing"

	"github.com/d56de/shrike/internal/core"
)

func TestInfo_NotDestructive(t *testing.T) {
	a := NewInfo()
	if a.Destructive() {
		t.Error("info must not be destructive")
	}
	if a.Key() != 'i' {
		t.Error("expected key 'i'")
	}
}

func TestInfo_ExecuteProducesOneResultPerTarget(t *testing.T) {
	a := NewInfo()
	targets := []core.ProcessInfo{{PID: 1}, {PID: 2}}
	results := a.Execute(context.Background(), targets)
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
}
