package detectors

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/d56de/shrike/internal/core"
)

type gpuSource struct {
	sample core.GPUSample
	err    error
}

func (s *gpuSource) Snapshot(context.Context) (core.GPUSample, error) { return s.sample, s.err }

// gpuFixture: the device reports the idle-artifact utilization (80 %); the
// real load lives in the processes' GPUPercent (70 + 25 = 95 %).
func gpuFixture() (*GPU, *gpuSource, []core.ProcessInfo) {
	u := 80.0
	s := &gpuSource{sample: core.GPUSample{At: time.Unix(1000, 0), Devices: []core.GPUDevice{{ID: 42, Name: "AGX", Utilization: &u}}}}
	p := []core.ProcessInfo{
		{PID: 1, Command: "Renderer", GPUPercent: 70},
		{PID: 2, Command: "Helper", GPUPercent: 25},
	}
	return NewGPU(s), s, p
}

func scan(g *GPU, s *gpuSource, p []core.ProcessInfo, cfg core.DetectorConfig, n int) []core.Finding {
	var f []core.Finding
	for i := 0; i < n; i++ {
		f = g.Detect(p, cfg)
		s.sample.At = s.sample.At.Add(15 * time.Second)
	}
	return f
}

func TestGPUIgnoresUtilizationArtifact(t *testing.T) {
	g, s, p := gpuFixture()
	for i := range p {
		p[i].GPUPercent = 0
	}
	for _, f := range scan(g, s, p, core.DetectorConfig{"threshold": 50.0}, 3) {
		if f.Severity >= core.SeverityMedium {
			t.Fatalf("artifact utilization produced a finding: %#v", f)
		}
	}
}

func TestGPUSustainedLoadAndCandidate(t *testing.T) {
	g, s, p := gpuFixture()
	f := scan(g, s, p, nil, 2)
	if len(f) != 1 || !f[0].System || f[0].Severity != core.SeverityMedium {
		t.Fatalf("unconfirmed: %#v", f)
	}
	f = scan(g, s, p, nil, 1)
	if len(f) != 3 || !f[0].System || f[0].Severity != core.SeverityHigh {
		t.Fatalf("confirmed: %#v", f)
	}
	if f[0].GPU.LoadPercent != 95 || f[1].Process.PID != 1 || f[2].Process.PID != 2 {
		t.Fatalf("load/candidates: %#v", f)
	}
	if f[1].GPU.LoadPercent != 70 {
		t.Fatalf("candidate load = %v, want 70", f[1].GPU.LoadPercent)
	}
}

func TestGPUResetsOnLowErrorAndLongGap(t *testing.T) {
	for _, reset := range []string{"low", "error", "gap", "backwards"} {
		t.Run(reset, func(t *testing.T) {
			g, s, p := gpuFixture()
			scan(g, s, p, nil, 2)
			low := []core.ProcessInfo{{PID: 1, Command: "Renderer", GPUPercent: 5}}
			switch reset {
			case "low":
				g.Detect(low, nil)
			case "error":
				s.err = errors.New("unavailable")
				g.Detect(p, nil)
				s.err = nil
			case "gap":
				s.sample.At = s.sample.At.Add(3 * time.Minute)
			case "backwards":
				s.sample.At = time.Unix(900, 0)
			}
			s.sample.At = s.sample.At.Add(5 * time.Second)
			f := g.Detect(p, nil)
			if len(f) != 1 || f[0].Severity != core.SeverityMedium {
				t.Fatalf("stale high state: %#v", f)
			}
		})
	}
}

func TestGPUIgnoredAndZombieNotCandidates(t *testing.T) {
	g, s, p := gpuFixture()
	p[0].State = core.StateZombie
	f := scan(g, s, p, core.DetectorConfig{"ignore": []string{"Helper"}}, 3)
	if len(f) != 1 || !f[0].System || f[0].Severity != core.SeverityHigh || f[0].GPU.LoadPercent != 95 {
		t.Fatalf("want only the system warning with full load: %#v", f)
	}
}

func TestGPUUnsupportedIsVisible(t *testing.T) {
	g, s, _ := gpuFixture()
	s.sample.Devices = nil
	f := g.Detect(nil, nil)
	if len(f) != 1 || !f[0].System || f[0].Severity != core.SeverityLow {
		t.Fatalf("unsupported: %#v", f)
	}
}

func TestGPUCustomThresholdAndDuration(t *testing.T) {
	g, s, p := gpuFixture()
	cfg := core.DetectorConfig{"threshold": 94.0, "min_duration": 45 * time.Second}
	for i := 0; i < 4; i++ {
		f := g.Detect(p, cfg)
		want := core.SeverityMedium
		if i == 3 {
			want = core.SeverityHigh
		}
		if len(f) == 0 || f[0].Severity != want || f[0].GPU.DeviceID != 42 {
			t.Fatalf("sample %d: %#v", i, f)
		}
		s.sample.At = s.sample.At.Add(15 * time.Second)
	}
	cfg["threshold"] = 99.0
	if f := g.Detect(p, cfg); len(f) != 0 {
		t.Fatalf("custom threshold ignored: %#v", f)
	}
}
