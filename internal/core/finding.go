package core

// Severity ranks how suspicious a finding is.
type Severity int

// Severity values in ascending order.
const (
	SeverityLow Severity = iota
	SeverityMedium
	SeverityHigh
	SeverityCritical
)

// String returns the lowercase label for the severity.
func (s Severity) String() string {
	switch s {
	case SeverityLow:
		return "low"
	case SeverityMedium:
		return "medium"
	case SeverityHigh:
		return "high"
	case SeverityCritical:
		return "critical"
	default:
		return "unknown"
	}
}

// Finding is what a Detector produces for a single suspicious process.
type Finding struct {
	Process  ProcessInfo // full copy so UI can render after a kill
	Detector string      // "runaway" | "zombie" | "herd"
	Severity Severity
	Score    float64 // internal sort key; not displayed in main UI
	Reason   string  // human-readable one-liner
	Group    *HerdGroup
}

// HerdGroup is attached to herd findings; non-nil only for detector=="herd".
type HerdGroup struct {
	Parent   ProcessInfo
	Children []ProcessInfo
	TotalCPU float64
	TotalRSS uint64
}
