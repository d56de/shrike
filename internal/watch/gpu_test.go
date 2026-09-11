package watch

import (
	"strings"
	"testing"

	"github.com/d56de/shrike/internal/core"
)

func TestGPUNotificationsSeparateDevicesAndEscalate(t *testing.T) {
	w := NewWatcher(core.SeverityHigh)
	f := core.Finding{Detector: "gpu", System: true, Severity: core.SeverityMedium, Process: core.ProcessInfo{Command: "GPU"}, GPU: &core.GPUFinding{DeviceID: 42}, Reason: "repeated overload"}
	if len(w.Decide([]core.Finding{f})) != 0 {
		t.Fatal("notified transient")
	}
	f.Severity = core.SeverityHigh
	n := w.Decide([]core.Finding{f})
	if len(n) != 1 || strings.Contains(n[0].Message, "PID 0") {
		t.Fatalf("invalid system notification: %#v", n)
	}
	if len(w.Decide([]core.Finding{f})) != 0 {
		t.Fatal("duplicate notification")
	}
	second := f
	second.GPU = &core.GPUFinding{DeviceID: 43}
	if len(w.Decide([]core.Finding{f, second})) != 1 {
		t.Fatal("second device hidden by deduplication")
	}
}
