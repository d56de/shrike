package core

import (
	"context"
	"time"
)

// GPUSnapshotter provides optional GPU telemetry without changing process snapshots.
type GPUSnapshotter interface {
	Snapshot(context.Context) (GPUSample, error)
}

// GPUSample is one observation; absent utilization means unknown, not zero.
type GPUSample struct {
	At      time.Time
	Devices []GPUDevice
}

// GPUDevice keeps clients associated with the GPU whose load was measured.
type GPUDevice struct {
	ID          uint64
	Name        string
	Utilization *float64
	Clients     []GPUClient
}

// GPUClient is a driver client with a cumulative, driver-defined GPU-time counter.
// Ticks must not be interpreted as wall-clock nanoseconds or GPU percent.
type GPUClient struct {
	ID       uint64
	PID      int
	Ticks    uint64
	Channels int // AppUsage entry count; changes invalidate a client delta
}

// GPUFinding preserves the measurements behind a GPU finding in JSON/history.
type GPUFinding struct {
	DeviceID        uint64   `json:"device_id"`
	Device          string   `json:"device"`
	Utilization     *float64 `json:"utilization_percent,omitempty"`
	ObservedSeconds float64  `json:"observed_seconds"`
	Samples         int      `json:"samples"`
	DeltaTicks      uint64   `json:"delta_ticks,omitempty"`
	IntervalSeconds float64  `json:"interval_seconds,omitempty"`
}

// ContextDetector is optionally implemented by detectors that perform bounded I/O.
type ContextDetector interface {
	DetectContext(context.Context, []ProcessInfo, DetectorConfig) []Finding
}
