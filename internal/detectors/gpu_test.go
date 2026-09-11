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

func gpuFixture() (*GPU, *gpuSource, []core.ProcessInfo) {
	u := 97.0
	s := &gpuSource{sample: core.GPUSample{At: time.Unix(1000, 0), Devices: []core.GPUDevice{{
		ID: 42, Name: "AGX", Utilization: &u,
		Clients: []core.GPUClient{{ID: 99, PID: 123, Ticks: 100}},
	}}}}
	p := []core.ProcessInfo{{PID: 123, Command: "Renderer", StartedAt: time.Unix(500, 0)}}
	return NewGPU(s), s, p
}

func TestGPUSustainedOverloadAndCandidate(t *testing.T) {
	g, s, p := gpuFixture()
	f := g.Detect(p, nil)
	if len(f) != 1 || !f[0].System || f[0].Severity != core.SeverityMedium {
		t.Fatalf("initial: %#v", f)
	}
	s.sample.At = s.sample.At.Add(15 * time.Second)
	s.sample.Devices[0].Clients[0].Ticks = 200
	f = g.Detect(p, nil)
	if len(f) != 1 || f[0].Severity != core.SeverityMedium {
		t.Fatalf("unconfirmed: %#v", f)
	}
	s.sample.At = s.sample.At.Add(15 * time.Second)
	s.sample.Devices[0].Clients[0].Ticks = 400
	f = g.Detect(p, nil)
	if len(f) != 2 || !f[0].System || f[0].Severity != core.SeverityHigh || f[1].Process.PID != 123 || f[1].System {
		t.Fatalf("confirmed: %#v", f)
	}
	if f[1].GPU.DeltaTicks != 200 || f[1].GPU.IntervalSeconds != 15 {
		t.Fatalf("measurement: %#v", f[1].GPU)
	}
}

func TestGPUResetsOnLowMissingErrorAndLongGap(t *testing.T) {
	for _, reset := range []string{"low", "missing", "error", "gap", "backwards"} {
		t.Run(reset, func(t *testing.T) {
			g, s, p := gpuFixture()
			g.Detect(p, nil)
			s.sample.At = s.sample.At.Add(15 * time.Second)
			g.Detect(p, nil)
			u := 97.0
			s.sample.At = s.sample.At.Add(15 * time.Second)
			switch reset {
			case "low":
				u = 10
				s.sample.Devices[0].Utilization = &u
			case "missing":
				s.sample.Devices[0].Utilization = nil
			case "error":
				s.err = errors.New("unavailable")
			case "gap":
				s.sample.At = s.sample.At.Add(3 * time.Minute)
			case "backwards":
				s.sample.At = time.Unix(900, 0)
			}
			g.Detect(p, nil)
			s.err = nil
			u = 97
			s.sample.Devices[0].Utilization = &u
			s.sample.At = s.sample.At.Add(5 * time.Second)
			f := g.Detect(p, nil)
			if len(f) != 1 || f[0].Severity != core.SeverityMedium {
				t.Fatalf("stale high state: %#v", f)
			}
		})
	}
}

func TestGPUDoesNotAttributeResetChurnReuseExitOrIgnoredProcess(t *testing.T) {
	for _, change := range []string{"reset", "client", "channels", "pid", "exit", "ignore", "no-growth"} {
		t.Run(change, func(t *testing.T) {
			g, s, p := gpuFixture()
			g.Detect(p, nil)
			s.sample.At = s.sample.At.Add(15 * time.Second)
			g.Detect(p, nil)
			s.sample.At = s.sample.At.Add(15 * time.Second)
			s.sample.Devices[0].Clients[0].Ticks = 500
			cfg := core.DetectorConfig{}
			switch change {
			case "reset":
				s.sample.Devices[0].Clients[0].Ticks = 10
			case "client":
				s.sample.Devices[0].Clients[0].ID = 101
			case "channels":
				s.sample.Devices[0].Clients[0].Channels = 2
			case "pid":
				p[0].StartedAt = time.Unix(999, 0)
			case "exit":
				p = nil
			case "ignore":
				cfg["ignore"] = []string{"Renderer"}
			case "no-growth":
				s.sample.Devices[0].Clients[0].Ticks = 100
			}
			f := g.Detect(p, cfg)
			if len(f) != 1 || !f[0].System || f[0].Severity != core.SeverityHigh {
				t.Fatalf("incorrect candidate or lost warning: %#v", f)
			}
		})
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

func TestGPUCustomThresholdAndDurationKeepDevicesSeparate(t *testing.T) {
	g, s, p := gpuFixture()
	low := 50.0
	s.sample.Devices = append(s.sample.Devices, core.GPUDevice{ID: 43, Name: "Second", Utilization: &low})
	cfg := core.DetectorConfig{"threshold": 96.0, "min_duration": 45 * time.Second}
	for i := 0; i < 4; i++ {
		f := g.Detect(p, cfg)
		if len(f) != 1 || f[0].GPU.DeviceID != 42 {
			t.Fatalf("device mix-up: %#v", f)
		}
		want := core.SeverityMedium
		if i == 3 {
			want = core.SeverityHigh
		}
		if f[0].Severity != want {
			t.Fatalf("sample %d: severity=%v", i, f[0].Severity)
		}
		s.sample.At = s.sample.At.Add(15 * time.Second)
	}
	cfg["threshold"] = 99.0
	if f := g.Detect(p, cfg); len(f) != 0 {
		t.Fatalf("custom threshold ignored: %#v", f)
	}
}
