package actions

import (
	"context"
	"os"
	"testing"
)

func TestFormatBytes(t *testing.T) {
	cases := []struct {
		in   uint64
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{2_500_000, "2.4 MB"},
		{3_500_000_000, "3.3 GB"},
	}
	for _, c := range cases {
		if got := FormatBytes(c.in); got != c.want {
			t.Errorf("FormatBytes(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFormatAncestors(t *testing.T) {
	if got := FormatAncestors(nil); got != "—" {
		t.Errorf("empty ancestry should render as em-dash, got %q", got)
	}
	got := FormatAncestors([]Ancestor{
		{PID: 1, Command: "launchd"},
		{PID: 456, Command: ""}, // missing name → "?"
		{PID: 789, Command: "zsh"},
	})
	want := "launchd(1) → ?(456) → zsh(789)"
	if got != want {
		t.Errorf("FormatAncestors mismatch:\n got=%q\nwant=%q", got, want)
	}
}

// TestFetchInfo_ReturnsForCurrentProcess is a smoke test: the fetcher must
// return without crashing for our own PID, and at least one of the lightweight
// fields (Cwd, Threads, Ancestors) should be populated.
func TestFetchInfo_ReturnsForCurrentProcess(t *testing.T) {
	d := FetchInfo(context.Background(), os.Getpid())
	if d.PID != os.Getpid() {
		t.Errorf("PID echo mismatch: got %d want %d", d.PID, os.Getpid())
	}
	if d.Threads == 0 && d.Cwd == "" && len(d.Ancestors) == 0 {
		t.Errorf("expected at least one of Threads/Cwd/Ancestors to be set, got %+v", d)
	}
}

// TestFetchInfo_DeadPIDDoesNotPanic — using an obviously-dead PID returns a
// struct with a note, never a panic. We pick 1<<22 (≈ 4M) which is outside
// the macOS PID range (max 99999) so it cannot collide with a real process.
func TestFetchInfo_DeadPIDDoesNotPanic(t *testing.T) {
	d := FetchInfo(context.Background(), 1<<22)
	if d.PID != 1<<22 {
		t.Errorf("PID echo mismatch: got %d", d.PID)
	}
	// Threads and Cwd should remain zero/empty for a dead PID.
	if d.Threads != 0 || d.Cwd != "" {
		t.Errorf("expected zero-valued fields for dead PID, got %+v", d)
	}
}
