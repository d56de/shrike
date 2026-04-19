package history

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/d56de/shrike/internal/core"
)

func TestWriter_AppendsRunAndFindings(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tmp)

	w, err := NewWriter()
	if err != nil {
		t.Fatal(err)
	}

	run := RunMeta{TS: time.Now(), Mode: "doctor", ProcsScanned: 47, DurationMS: 234}
	findings := []core.Finding{
		{
			Detector: "runaway", Severity: core.SeverityHigh, Score: 151,
			Process: core.ProcessInfo{
				PID: 93187, Command: "Chrome Helper",
				CPUPercent: 99.1, RSS: 148897792, ElapsedTime: 23 * time.Hour,
			},
		},
	}
	if err := w.AppendRun(run, findings); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(tmp, "shrike", "history.jsonl")
	f, err := os.Open(path) //nolint:gosec // test uses t.TempDir path
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines (1 run + 1 finding), got %d", len(lines))
	}
	if !strings.Contains(lines[0], `"_type":"run"`) {
		t.Errorf("first line should be run meta: %s", lines[0])
	}
	if !strings.Contains(lines[1], `"_type":"finding"`) {
		t.Errorf("second line should be finding: %s", lines[1])
	}

	var got map[string]any
	if err := json.Unmarshal([]byte(lines[1]), &got); err != nil {
		t.Fatal(err)
	}
	if int(got["pid"].(float64)) != 93187 {
		t.Errorf("expected pid 93187")
	}
}
