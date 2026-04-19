package actions

import (
	"context"
	"errors"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/d56de/shrike/internal/core"
)

type fakeKiller struct {
	sent      []syscall.Signal
	dieAfter  int // how many signals before the fake process "dies"
	notPermit bool
}

func (f *fakeKiller) Signal(_ int, sig os.Signal) error {
	if f.notPermit {
		return errors.New("operation not permitted")
	}
	ssig, _ := sig.(syscall.Signal)
	f.sent = append(f.sent, ssig)
	return nil
}

func (f *fakeKiller) Alive(_ int) bool {
	return len(f.sent) < f.dieAfter
}

func TestKill_TERMOnly_WhenProcessDies(t *testing.T) {
	k := &fakeKiller{dieAfter: 1}
	a := &Kill{Killer: k, EscalateAfter: 10 * time.Millisecond}

	results := a.Execute(context.Background(), []core.ProcessInfo{{PID: 42}})

	if len(results) != 1 || results[0].Err != nil {
		t.Fatalf("expected success, got %+v", results)
	}
	if len(k.sent) != 1 || k.sent[0] != syscall.SIGTERM {
		t.Errorf("expected 1 SIGTERM, got %+v", k.sent)
	}
}

func TestKill_EscalatesToKILL_IfStillAlive(t *testing.T) {
	k := &fakeKiller{dieAfter: 2}
	a := &Kill{Killer: k, EscalateAfter: 10 * time.Millisecond}

	a.Execute(context.Background(), []core.ProcessInfo{{PID: 42}})

	if len(k.sent) != 2 {
		t.Fatalf("expected 2 signals, got %d", len(k.sent))
	}
	if k.sent[1] != syscall.SIGKILL {
		t.Errorf("expected SIGKILL after escalation, got %v", k.sent[1])
	}
}

func TestKill_NotPermitted(t *testing.T) {
	k := &fakeKiller{notPermit: true}
	a := &Kill{Killer: k, EscalateAfter: 10 * time.Millisecond}

	results := a.Execute(context.Background(), []core.ProcessInfo{{PID: 42}})

	if results[0].Err == nil {
		t.Error("expected error, got nil")
	}
}
