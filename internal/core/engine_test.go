package core

import (
	"context"
	"sync"
	"testing"
)

type fakeSnapshotter struct {
	procs []ProcessInfo
	err   error
}

type contextualTestDetector struct {
	fakeDetector
	expected context.Context
	received bool
}

func (d *contextualTestDetector) DetectContext(ctx context.Context, _ []ProcessInfo, _ DetectorConfig) []Finding {
	d.received = ctx == d.expected
	return nil
}

func TestEnginePassesContextToIOAndCanIgnoreDuringRun(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d := &contextualTestDetector{fakeDetector: fakeDetector{name: "gpu"}, expected: ctx}
	e := &Engine{Snapshotter: fakeSnapshotter{}, Detectors: []Detector{d}}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 50 {
			e.Ignore("gpu", "Renderer")
		}
	}()
	for range 50 {
		if _, err := e.Run(ctx); err != nil {
			t.Fatal(err)
		}
	}
	wg.Wait()
	if !d.received {
		t.Fatal("context was not forwarded")
	}
	ignore, _ := e.Configs["gpu"]["ignore"].([]string)
	if len(ignore) != 1 {
		t.Fatalf("ignore not idempotent: %v", ignore)
	}
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
