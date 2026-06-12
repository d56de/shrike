package notify

import (
	"errors"
	"reflect"
	"testing"
)

type recorder struct {
	name string
	args []string
}

func newSystemWith(hasTN bool, rec *recorder) *System {
	return &System{
		lookPath: func(string) (string, error) {
			if hasTN {
				return "/opt/homebrew/bin/terminal-notifier", nil
			}
			return "", errors.New("not found")
		},
		run: func(name string, args ...string) error {
			rec.name = name
			rec.args = args
			return nil
		},
	}
}

func TestNotify_TerminalNotifier(t *testing.T) {
	var rec recorder
	s := newSystemWith(true, &rec)
	_ = s.Notify(Notification{Title: "T", Subtitle: "S", Message: "M", Group: "g"})
	if rec.name != "terminal-notifier" {
		t.Fatalf("name = %q, want terminal-notifier", rec.name)
	}
	want := []string{"-title", "T", "-message", "M", "-subtitle", "S", "-group", "g"}
	if !reflect.DeepEqual(rec.args, want) {
		t.Errorf("args = %v, want %v", rec.args, want)
	}
}

func TestNotify_TerminalNotifier_OmitsEmptyOptionals(t *testing.T) {
	var rec recorder
	s := newSystemWith(true, &rec)
	_ = s.Notify(Notification{Title: "T", Message: "M"})
	want := []string{"-title", "T", "-message", "M"}
	if !reflect.DeepEqual(rec.args, want) {
		t.Errorf("args = %v, want %v", rec.args, want)
	}
}

func TestNotify_OsascriptFallback(t *testing.T) {
	var rec recorder
	s := newSystemWith(false, &rec)
	_ = s.Notify(Notification{Title: "T", Subtitle: "S", Message: "M"})
	if rec.name != "osascript" {
		t.Fatalf("name = %q, want osascript", rec.name)
	}
	hasSep := false
	for _, a := range rec.args {
		if a == "--" {
			hasSep = true
		}
	}
	if !hasSep {
		t.Fatal("expected a -- separator before the values")
	}
	if got := rec.args[len(rec.args)-3:]; !reflect.DeepEqual(got, []string{"M", "T", "S"}) {
		t.Errorf("trailing argv = %v, want [M T S]", got)
	}
}

func TestNotify_OsascriptFallback_NoSubtitle(t *testing.T) {
	var rec recorder
	s := newSystemWith(false, &rec)
	_ = s.Notify(Notification{Title: "T", Message: "M"})
	if got := rec.args[len(rec.args)-2:]; !reflect.DeepEqual(got, []string{"M", "T"}) {
		t.Errorf("trailing argv = %v, want [M T]", got)
	}
}
