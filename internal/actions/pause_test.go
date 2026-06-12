package actions

import (
	"context"
	"syscall"
	"testing"

	"github.com/d56de/shrike/internal/core"
)

func TestPause_StopsRunningProcess(t *testing.T) {
	k := &fakeKiller{}
	p := Pause{Killer: k}

	res := p.Execute(context.Background(), []core.ProcessInfo{{PID: 42, State: core.StateRunning}})

	if len(res) != 1 || res[0].Err != nil || res[0].Message != "paused" {
		t.Fatalf("expected paused, got %+v", res)
	}
	if len(k.sent) != 1 || k.sent[0] != syscall.SIGSTOP {
		t.Errorf("expected one SIGSTOP, got %+v", k.sent)
	}
}

func TestPause_ResumesStoppedProcess(t *testing.T) {
	k := &fakeKiller{}
	p := Pause{Killer: k}

	res := p.Execute(context.Background(), []core.ProcessInfo{{PID: 42, State: core.StateStopped}})

	if res[0].Message != "resumed" {
		t.Fatalf("expected resumed, got %+v", res)
	}
	if len(k.sent) != 1 || k.sent[0] != syscall.SIGCONT {
		t.Errorf("expected one SIGCONT, got %+v", k.sent)
	}
}

func TestPause_SkipsZombie(t *testing.T) {
	k := &fakeKiller{}
	p := Pause{Killer: k}

	res := p.Execute(context.Background(), []core.ProcessInfo{{PID: 42, State: core.StateZombie}})

	if res[0].Message != "skipped (zombie)" {
		t.Errorf("expected skip, got %+v", res)
	}
	if len(k.sent) != 0 {
		t.Errorf("expected no signal sent, got %+v", k.sent)
	}
}

func TestPause_ReportsPermissionError(t *testing.T) {
	k := &fakeKiller{notPermit: true}
	p := Pause{Killer: k}

	res := p.Execute(context.Background(), []core.ProcessInfo{{PID: 42, State: core.StateRunning}})

	if res[0].Err == nil {
		t.Error("expected error, got nil")
	}
	if res[0].Message != "not permitted" {
		t.Errorf("expected 'not permitted', got %q", res[0].Message)
	}
}
