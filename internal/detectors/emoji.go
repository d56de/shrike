package detectors

// Emoji returns the glyph for a detector name, matching each detector's Emoji()
// method, or "•" for an unknown name.
func Emoji(name string) string {
	switch name {
	case "runaway":
		return "🔥"
	case "zombie":
		return "🧟"
	case "herd":
		return "👥"
	case "memleak":
		return "🧠"
	default:
		return "•"
	}
}
