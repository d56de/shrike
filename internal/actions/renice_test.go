package actions

import (
	"context"
	"errors"
	"testing"

	"github.com/d56de/shrike/internal/core"
)

type fakeRenicer struct {
	calls []int
	err   error
}

func (f *fakeRenicer) Renice(pid, _ int) error {
	if f.err != nil {
		return f.err
	}
	f.calls = append(f.calls, pid)
	return nil
}

func TestRenice_SuccessPath(t *testing.T) {
	r := &fakeRenicer{}
	a := &Renice{Renicer: r, Priority: 10}
	results := a.Execute(context.Background(), []core.ProcessInfo{{PID: 42}})
	if len(r.calls) != 1 || r.calls[0] != 42 {
		t.Errorf("expected Renice on PID 42, got %v", r.calls)
	}
	if results[0].Err != nil {
		t.Errorf("unexpected error: %v", results[0].Err)
	}
}

func TestRenice_PermissionError(t *testing.T) {
	r := &fakeRenicer{err: errors.New("not permitted")}
	a := &Renice{Renicer: r, Priority: 10}
	results := a.Execute(context.Background(), []core.ProcessInfo{{PID: 42}})
	if results[0].Err == nil {
		t.Error("expected error")
	}
}
