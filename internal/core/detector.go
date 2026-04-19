package core

// DetectorConfig is a generic container populated by the loader from the
// relevant [section] in config.toml. Each detector knows how to read its own
// keys (type-asserted or unmarshaled inside the detector).
type DetectorConfig map[string]any

// Detector inspects a snapshot and emits Findings for suspicious processes.
type Detector interface {
	Name() string  // "runaway"
	Emoji() string // "🔥"
	Detect(snap []ProcessInfo, cfg DetectorConfig) []Finding
}
