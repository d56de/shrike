package detectors

import "testing"

func TestEmoji(t *testing.T) {
	cases := map[string]string{
		"runaway": "🔥",
		"zombie":  "🧟",
		"herd":    "👥",
		"memleak": "🧠",
		"nope":    "•",
	}
	for name, want := range cases {
		if got := Emoji(name); got != want {
			t.Errorf("Emoji(%q) = %q, want %q", name, got, want)
		}
	}
}
