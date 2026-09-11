package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/d56de/shrike/internal/config"
	"github.com/d56de/shrike/internal/core"
	"github.com/d56de/shrike/internal/history"
)

func TestBuildEngineGPUSelectionAndDisable(t *testing.T) {
	c := config.DefaultConfig()
	e := buildEngine(c, []string{"gpu"})
	if len(e.Detectors) != 1 || e.Detectors[0].Name() != "gpu" {
		t.Fatalf("GPU not registered: %#v", e.Detectors)
	}
	c.GPU.Enabled = false
	e = buildEngine(c, []string{"gpu"})
	if len(e.Detectors) != 0 {
		t.Fatal("disabled GPU still runs")
	}
}

func TestGPULogShowsReasonWithoutMissingProcessFields(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	w, err := history.NewWriter()
	if err != nil {
		t.Fatal(err)
	}
	err = w.AppendRun(history.RunMeta{TS: time.Now()}, []core.Finding{{Detector: "gpu", System: true, Reason: "GPU 97% repeated overload"}})
	if err != nil {
		t.Fatal(err)
	}
	if err = w.Close(); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	logCmd.SetOut(&out)
	t.Cleanup(func() { logCmd.SetOut(nil) })
	if err = logCmd.RunE(logCmd, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "GPU 97% repeated overload") || strings.Contains(out.String(), "PID <nil>") || strings.Contains(out.String(), "%!") {
		t.Fatalf("invalid log: %s", out.String())
	}
}

func TestGPUJSONPreservesScopeAndRawCounter(t *testing.T) {
	u := 97.0
	f := core.Finding{Detector: "gpu", System: true, GPU: &core.GPUFinding{DeviceID: 42, Utilization: &u, DeltaTicks: 200}}
	b, err := json.Marshal(findingToJSON(time.Now(), f))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err = json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got["scope"] != "system" || got["process"] != nil {
		t.Fatalf("fake process: %s", b)
	}
	meta, ok := got["gpu"].(map[string]any)
	if !ok || meta["device_id"] != float64(42) || meta["delta_ticks"] != float64(200) {
		t.Fatalf("lost GPU metadata: %s", b)
	}
}
