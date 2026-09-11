package history

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/d56de/shrike/internal/core"
)

func TestGPUHistoryPreservesSystemMetadata(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	w, err := NewWriter()
	if err != nil {
		t.Fatal(err)
	}
	err = w.AppendRun(RunMeta{}, []core.Finding{{Detector: "gpu", System: true, GPU: &core.GPUFinding{DeviceID: 42, DeltaTicks: 200}}})
	if err != nil {
		t.Fatal(err)
	}
	if err = w.Close(); err != nil {
		t.Fatal(err)
	}
	p, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var row map[string]any
	if err = json.Unmarshal([]byte(strings.Split(string(b), "\n")[1]), &row); err != nil {
		t.Fatal(err)
	}
	if row["scope"] != "system" || row["gpu"] == nil {
		t.Fatalf("metadata lost: %s", b)
	}
	if _, ok := row["pid"]; ok {
		t.Fatalf("system warning has a process PID: %s", b)
	}
}
