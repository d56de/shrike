package history

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/d56de/shrike/internal/core"
)

// RunMeta is the header record emitted at the start of each doctor/watch run.
type RunMeta struct {
	TS           time.Time `json:"ts"`
	Mode         string    `json:"mode"`
	ProcsScanned int       `json:"procs_scanned"`
	DurationMS   int64     `json:"duration_ms"`
}

// Writer appends run-meta and finding records to the JSONL history file.
type Writer struct {
	mu   sync.Mutex
	path string
	f    *os.File
}

// NewWriter opens the history file for append, creating parent directories as
// needed.
func NewWriter() (*Writer, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, fmt.Errorf("mkdir state dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644) //nolint:gosec // path is derived from XDG_STATE_HOME or user home
	if err != nil {
		return nil, fmt.Errorf("open history: %w", err)
	}
	return &Writer{path: path, f: f}, nil
}

// AppendRun writes one run-meta line followed by one line per finding.
func (w *Writer) AppendRun(run RunMeta, findings []core.Finding) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if err := writeLine(w.f, map[string]any{
		"_type":         "run",
		"ts":            run.TS.UTC().Format(time.RFC3339),
		"mode":          run.Mode,
		"procs_scanned": run.ProcsScanned,
		"duration_ms":   run.DurationMS,
	}); err != nil {
		return err
	}
	for _, f := range findings {
		rec := map[string]any{
			"_type":     "finding",
			"ts":        run.TS.UTC().Format(time.RFC3339),
			"detector":  f.Detector,
			"severity":  f.Severity.String(),
			"score":     f.Score,
			"reason":    f.Reason,
			"pid":       f.Process.PID,
			"command":   f.Process.Command,
			"cpu":       f.Process.CPUPercent,
			"rss":       f.Process.RSS,
			"elapsed_s": int64(f.Process.ElapsedTime.Seconds()),
		}
		if err := writeLine(w.f, rec); err != nil {
			return err
		}
	}
	return nil
}

// Close releases the underlying file handle.
func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.f.Close()
}

func writeLine(f *os.File, rec map[string]any) error {
	data, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	return nil
}
