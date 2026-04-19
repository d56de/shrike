package core

import (
	"context"
	"testing"
)

type fakeSnapshotter struct {
	procs []ProcessInfo
	err   error
}

func (f fakeSnapshotter) Snapshot(_ context.Context) ([]ProcessInfo, error) {
	return f.procs, f.err
}

type fakeDetector struct {
	name     string
	findings []Finding
}

func (f fakeDetector) Name() string                                       { return f.name }
func (f fakeDetector) Emoji() string                                      { return "•" }
func (f fakeDetector) Detect(_ []ProcessInfo, _ DetectorConfig) []Finding { return f.findings }

func TestEngine_RunSortsBySeverityThenScore(t *testing.T) {
	procs := []ProcessInfo{{PID: 1}}
	d1 := fakeDetector{name: "a", findings: []Finding{
		{Detector: "a", Severity: SeverityMedium, Score: 10},
	}}
	d2 := fakeDetector{name: "b", findings: []Finding{
		{Detector: "b", Severity: SeverityHigh, Score: 5},
		{Detector: "b", Severity: SeverityMedium, Score: 50},
	}}
	e := Engine{
		Snapshotter: fakeSnapshotter{procs: procs},
		Detectors:   []Detector{d1, d2},
	}
	findings, err := e.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 3 {
		t.Fatalf("expected 3 findings, got %d", len(findings))
	}
	if findings[0].Severity != SeverityHigh {
		t.Errorf("first finding should be High, got %v", findings[0].Severity)
	}
	if findings[1].Score != 50 {
		t.Errorf("second finding should be Medium/50, got Score=%v", findings[1].Score)
	}
}
