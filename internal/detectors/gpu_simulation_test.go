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

type simulationProcesses struct{ gpu *float64 }

func (s simulationProcesses) Snapshot(context.Context) ([]core.ProcessInfo, error) {
	return []core.ProcessInfo{{
		PID: 4242, Command: "Simulated Renderer", StartedAt: time.Unix(100, 0),
		CPUPercent: 0.1, GPUPercent: *s.gpu, State: core.StateRunning,
	}}, nil
}

func TestGPUSimulation(t *testing.T) {
	t.Run("spike_overload_attribution_recovery", func(t *testing.T) {
		source := &simulationGPU{}
		var gpu float64
		engine := &core.Engine{Snapshotter: simulationProcesses{gpu: &gpu}, Detectors: []core.Detector{detectors.NewGPU(source)}}
		notifications := watch.NewWatcher(core.SeverityHigh)
		steps := []struct {
			seconds       int
			load          float64
			findings      int
			severity      core.Severity
			notifications int
		}{
			{0, 12, 0, core.SeverityLow, 0},
			{5, 99, 1, core.SeverityMedium, 0},
			{10, 15, 0, core.SeverityLow, 0},
			{15, 98, 1, core.SeverityMedium, 0},
			{20, 99, 1, core.SeverityMedium, 0},
			{25, 97, 1, core.SeverityMedium, 0},
			{30, 99, 1, core.SeverityMedium, 0},
			{35, 98, 1, core.SeverityMedium, 0},
			{40, 99, 1, core.SeverityMedium, 0},
			{45, 99, 2, core.SeverityHigh, 1},
			{50, 98, 2, core.SeverityHigh, 0},
			{60, 12, 0, core.SeverityLow, 0},
			{65, 99, 1, core.SeverityMedium, 0},
			{80, 99, 1, core.SeverityMedium, 0},
			{95, 99, 2, core.SeverityHigh, 1},
		}
		for _, step := range steps {
			gpu = step.load
			// Device utilization pinned at the idle artifact: it must not matter.
			artifact := 80.0
			device := core.GPUDevice{ID: 42, Name: "Simulated GPU", Utilization: &artifact}
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
			if len(findings) == 2 && (findings[1].Process.PID != 4242 || findings[1].GPU.LoadPercent != step.load) {
				t.Fatalf("t=%ds: missing GPU candidate", step.seconds)
			}
			t.Logf("t=%02ds GPU=%2.0f%% status=%-6s findings=%d notifications=%d", step.seconds, step.load, status, len(findings), len(notes))
			if step.seconds == 45 {
				model := doctor.NewModel(findings, nil)
				model.Width, model.Height = 120, 40
				view := model.View()
				if !strings.Contains(view, "99.0% GPU · system") || !strings.Contains(view, "Simulated Renderer") || !strings.Contains(view, "PID 4242 · 99.0% GPU") {
					t.Fatalf("confirmed overload absent from real doctor view:\n%s", view)
				}
				t.Logf("notification (preview only): %s — %s", notes[0].Title, notes[0].Message)
			}
		}
	})
}
