package history

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReader_ReadsAll(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tmp)

	dir := filepath.Join(tmp, "shrike")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	content := `{"_type":"run","ts":"2026-04-19T08:53:12Z","mode":"doctor","procs_scanned":47,"duration_ms":234}
{"_type":"finding","ts":"2026-04-19T08:53:12Z","detector":"runaway","severity":"high","score":151.2,"pid":93187,"command":"Chrome Helper","cpu":99.1,"rss":148897792,"elapsed_s":85090}
garbage line — must be skipped
{"_type":"finding","ts":"2026-04-19T08:53:12Z","detector":"zombie","severity":"medium","score":10,"pid":47219,"command":"bash","cpu":0,"rss":0,"elapsed_s":8040}
`
	if err := os.WriteFile(filepath.Join(dir, "history.jsonl"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	records, err := Read(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 3 {
		t.Fatalf("expected 3 valid records, got %d", len(records))
	}
}

func TestReader_FiltersBySince(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tmp)

	dir := filepath.Join(tmp, "shrike")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	old := now.Add(-48 * time.Hour).UTC().Format(time.RFC3339)
	recent := now.Add(-1 * time.Hour).UTC().Format(time.RFC3339)
	content := strings.Join([]string{
		`{"_type":"run","ts":"` + old + `","mode":"doctor"}`,
		`{"_type":"run","ts":"` + recent + `","mode":"doctor"}`,
	}, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(dir, "history.jsonl"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	records, err := Read(24*time.Hour, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 (within 24h), got %d", len(records))
	}
}

func TestReader_FiltersByPID(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tmp)

	dir := filepath.Join(tmp, "shrike")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	content := `{"_type":"finding","ts":"2026-04-19T08:53:12Z","detector":"runaway","pid":1}
{"_type":"finding","ts":"2026-04-19T08:53:12Z","detector":"runaway","pid":2}
`
	if err := os.WriteFile(filepath.Join(dir, "history.jsonl"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	records, err := Read(0, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record for pid 2, got %d", len(records))
	}
}
