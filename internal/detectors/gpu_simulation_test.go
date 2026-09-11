package detectors_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/d56de/shrike/internal/core"
	"github.com/d56de/shrike/internal/detectors"
	"github.com/d56de/shrike/internal/tui/doctor"
	"github.com/d56de/shrike/internal/watch"
)

// These sources supply synthetic observations to the real engine, detector,
// notification policy and renderer. No OS sensor, notification or action runs.
type simulationGPU struct{ sample core.GPUSample }

func (s *simulationGPU) Snapshot(context.Context) (core.GPUSample, error) {
	return s.sample, nil
}

type simulationProcesses struct{}

func (simulationProcesses) Snapshot(context.Context) ([]core.ProcessInfo, error) {
	return []core.ProcessInfo{{
		PID: 4242, Command: "Simulated Renderer", StartedAt: time.Unix(100, 0),
		CPUPercent: 0.1, State: core.StateRunning,
	}}, nil
}

func TestGPUSimulation(t *testing.T) {
	t.Run("spike_overload_attribution_loss_recovery", func(t *testing.T) {
		source := &simulationGPU{}
		engine := &core.Engine{Snapshotter: simulationProcesses{}, Detectors: []core.Detector{detectors.NewGPU(source)}}
		notifications := watch.NewWatcher(core.SeverityHigh)
		steps := []struct {
			seconds       int
			load          float64
			clients       bool
			findings      int
			severity      core.Severity
			notifications int
		}{
			{0, 12, true, 0, core.SeverityLow, 0},
			{5, 99, true, 1, core.SeverityMedium, 0},
			{10, 15, true, 0, core.SeverityLow, 0},
			{15, 98, true, 1, core.SeverityMedium, 0},
			{20, 99, true, 1, core.SeverityMedium, 0},
			{25, 97, true, 1, core.SeverityMedium, 0},
			{30, 99, true, 1, core.SeverityMedium, 0},
			{35, 98, true, 1, core.SeverityMedium, 0},
			{40, 99, true, 1, core.SeverityMedium, 0},
			{45, 99, true, 2, core.SeverityHigh, 1},
			{50, 98, true, 2, core.SeverityHigh, 0},
			{55, 99, false, 1, core.SeverityHigh, 0},
			{60, 12, false, 0, core.SeverityLow, 0},
			{65, 99, true, 1, core.SeverityMedium, 0},
			{80, 99, true, 1, core.SeverityMedium, 0},
			{95, 99, true, 2, core.SeverityHigh, 1},
		}
		for _, step := range steps {
			load := step.load
			device := core.GPUDevice{ID: 42, Name: "Simulated GPU", Utilization: &load}
			if step.clients {
				device.Clients = []core.GPUClient{{ID: 99, PID: 4242, Ticks: 1000 + uint64(step.seconds)*1000, Channels: 1}}
			}
			source.sample = core.GPUSample{At: time.Unix(1000+int64(step.seconds), 0), Devices: []core.GPUDevice{device}}
			findings, err := engine.Run(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			notes := notifications.Decide(findings)
			if len(findings) != step.findings || len(notes) != step.notifications {
				t.Fatalf("t=%ds: findings=%d notifications=%d; want %d/%d", step.seconds, len(findings), len(notes), step.findings, step.notifications)
			}
			status := "clean"
			if len(findings) > 0 {
				if findings[0].Severity != step.severity || !findings[0].System {
					t.Fatalf("t=%ds: invalid system finding: %#v", step.seconds, findings[0])
				}
				status = findings[0].Severity.String()
			}
			if len(findings) == 2 && (findings[1].Process.PID != 4242 || findings[1].GPU.DeltaTicks == 0) {
				t.Fatalf("t=%ds: missing GPU candidate", step.seconds)
			}
			t.Logf("t=%02ds GPU=%2.0f%% status=%-6s findings=%d notifications=%d", step.seconds, step.load, status, len(findings), len(notes))
			if step.seconds == 45 {
				model := doctor.NewModel(findings, nil)
				model.Width, model.Height = 120, 40
				view := model.View()
				if !strings.Contains(view, "99.0% GPU") || !strings.Contains(view, "Simulated Renderer") || !strings.Contains(view, "GPU time +5000 ticks") {
					t.Fatalf("confirmed overload absent from real doctor view:\n%s", view)
				}
				t.Logf("notification (preview only): %s — %s", notes[0].Title, notes[0].Message)
				for _, line := range strings.Split(view, "\n") {
					if strings.Contains(line, "Simulated") || strings.Contains(line, "observations") || strings.Contains(line, "GPU-active") {
						t.Log(line)
					}
				}
			}
		}
	})

	t.Run("unknown_cause_still_warns", func(t *testing.T) {
		source := &simulationGPU{}
		engine := &core.Engine{Snapshotter: simulationProcesses{}, Detectors: []core.Detector{detectors.NewGPU(source)}}
		notifications := watch.NewWatcher(core.SeverityHigh)
		for _, seconds := range []int{0, 15, 30} {
			load := 99.0
			source.sample = core.GPUSample{At: time.Unix(2000+int64(seconds), 0), Devices: []core.GPUDevice{{ID: 42, Name: "Simulated GPU", Utilization: &load}}}
			findings, err := engine.Run(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			notes := notifications.Decide(findings)
			if len(findings) != 1 || !findings[0].System {
				t.Fatalf("invented process or lost warning: %#v", findings)
			}
			if seconds == 30 {
				if findings[0].Severity != core.SeverityHigh || len(notes) != 1 {
					t.Fatal("unknown cause prevented overload notification")
				}
				t.Logf("t=30s without process counters: %s — %s", notes[0].Title, notes[0].Message)
			}
		}
	})
}
