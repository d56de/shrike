package doctor

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/d56de/shrike/internal/actions"
	"github.com/d56de/shrike/internal/core"
)

func TestSystemGPUFindingCannotDispatchProcessActions(t *testing.T) {
	for _, key := range []rune{'k', 'K', 'r', 'p', 's', 'i', 'I', ' '} {
		t.Run(string(key), func(t *testing.T) {
			m := NewModel([]core.Finding{{Detector: "gpu", System: true, Process: core.ProcessInfo{Command: "GPU"}}}, []core.Action{actions.NewKill(), actions.NewKillImmediate(), actions.NewRenice()})
			m.PauseAction = stubPause{}
			m.IgnorePath = filepath.Join(t.TempDir(), "ignore.toml")
			next, cmd := m.Update(keyRunes(key))
			got := next.(Model)
			if cmd != nil || got.Mode != ModeList || len(got.Selected) != 0 {
				t.Fatalf("system action %c: mode=%v selected=%v cmd=%v", key, got.Mode, got.Selected, cmd != nil)
			}
			if len(got.selectedTargets()) != 0 || len(got.pauseTargets()) != 0 {
				t.Fatal("system row resolved to a process")
			}
		})
	}
}

func TestGPURowFitsNarrowTerminal(t *testing.T) {
	u := 99.0
	m := NewModel([]core.Finding{{Detector: "gpu", Process: core.ProcessInfo{PID: 123, Command: "A very long renderer process name"}, GPU: &core.GPUFinding{Utilization: &u, DeltaTicks: 999999999999, IntervalSeconds: 5}, Reason: "GPU-active candidate, not proof of a hung process"}}, nil)
	m.Width = 70
	m.Height = 35
	for _, line := range strings.Split(m.View(), "\n") {
		if lipgloss.Width(line) > 70 {
			t.Fatalf("row overflows: %q", line)
		}
	}
}

func TestGPURefreshPreservesSelectedDevice(t *testing.T) {
	first := core.Finding{Detector: "gpu", System: true, GPU: &core.GPUFinding{DeviceID: 42}}
	second := core.Finding{Detector: "gpu", System: true, GPU: &core.GPUFinding{DeviceID: 43}}
	third := core.Finding{Detector: "gpu", System: true, GPU: &core.GPUFinding{DeviceID: 44}}
	m := NewModel([]core.Finding{first, second, third}, nil)
	m.Cursor = 1
	next, _ := m.Update(RescanDoneMsg{Findings: []core.Finding{first, second, third}})
	if next.(Model).Cursor != 1 {
		t.Fatal("refresh lost GPU device focus")
	}
}

func TestGPUIgnoreSurvivesRescanInSameSession(t *testing.T) {
	m := NewModel([]core.Finding{{Detector: "gpu", Process: core.ProcessInfo{PID: 123, Command: "Renderer"}}}, nil)
	m.IgnorePath = filepath.Join(t.TempDir(), "ignore.toml")
	m.Engine = &core.Engine{Configs: map[string]core.DetectorConfig{"gpu": {"ignore": []string{}}}}
	next, _ := m.Update(keyRunes('I'))
	m = next.(Model)
	next, _ = m.Update(keyRunes('y'))
	m = next.(Model)
	ignore, _ := m.Engine.Configs["gpu"]["ignore"].([]string)
	if len(ignore) != 1 || ignore[0] != "Renderer" {
		t.Fatalf("rescan still sees old ignores: %v", ignore)
	}
}

func TestGPUIgnoreCannotHideSameNamedSystemWarning(t *testing.T) {
	m := NewModel([]core.Finding{
		{Detector: "gpu", System: true, Process: core.ProcessInfo{Command: "Renderer"}},
		{Detector: "gpu", Process: core.ProcessInfo{PID: 123, Command: "Renderer"}},
	}, nil)
	m = m.filterIgnored("gpu", "Renderer")
	if len(m.Findings) != 1 || !m.Findings[0].System {
		t.Fatal("candidate ignore hid the system warning")
	}
}

func TestGPUMixedSelectionTargetsOnlyRealProcesses(t *testing.T) {
	m := NewModel([]core.Finding{
		{Detector: "gpu", System: true, Process: core.ProcessInfo{Command: "GPU"}},
		{Detector: "gpu", Process: core.ProcessInfo{PID: 123, Command: "Renderer"}},
	}, nil)
	m.Selected = map[int]bool{0: true, 1: true}
	for _, targets := range [][]core.ProcessInfo{m.selectedTargets(), m.pauseTargets()} {
		if len(targets) != 1 || targets[0].PID != 123 {
			t.Fatalf("targets = %#v", targets)
		}
	}
	m.Cursor = 1
	next, _ := m.Update(keyRunes('I'))
	if next.(Model).Mode != ModeConfirmIgnore {
		t.Fatal("GPU candidate cannot be ignored")
	}
}

func TestGPURowShowsGPUAndReasonWithoutFakeCPUOrPID(t *testing.T) {
	u := 97.0
	m := NewModel([]core.Finding{{Detector: "gpu", System: true, Process: core.ProcessInfo{Command: "AGX"}, GPU: &core.GPUFinding{Utilization: &u}, Reason: "cause not established"}}, nil)
	m.Width = 120
	m.Height = 35
	view := m.View()
	if !strings.Contains(view, "97.0% GPU") || !strings.Contains(view, "cause not established") || strings.Contains(view, "PID 0") || strings.Contains(view, "0.0% CPU") {
		t.Fatalf("GPU data missing or misleading:\n%s", view)
	}
}
