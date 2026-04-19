package core

import "testing"

func TestProcessState_String(t *testing.T) {
	cases := []struct {
		s    ProcessState
		want string
	}{
		{StateRunning, "running"},
		{StateZombie, "zombie"},
		{StateUnknown, "unknown"},
		{ProcessState(999), "unknown"},
	}
	for _, c := range cases {
		if got := c.s.String(); got != c.want {
			t.Errorf("%d.String() = %q, want %q", c.s, got, c.want)
		}
	}
}
