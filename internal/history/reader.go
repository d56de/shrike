package history

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"time"
)

// Record is one parsed JSONL line. All fields live in Raw for flexibility;
// Type and TS are surfaced for easy filtering.
type Record struct {
	Type string         `json:"_type"`
	TS   time.Time      `json:"ts"`
	Raw  map[string]any `json:"-"`
}

// Read returns all records in the history file, optionally filtered.
// since=0 means no time filter; pid=0 means no pid filter.
// Corrupt lines are skipped silently.
func Read(since time.Duration, pid int) ([]Record, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path) //nolint:gosec // path derived from XDG_STATE_HOME or user home
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	cutoff := time.Time{}
	if since > 0 {
		cutoff = time.Now().Add(-since)
	}

	var out []Record
	sc := bufio.NewScanner(f)
	buf := make([]byte, 1024*1024)
	sc.Buffer(buf, 1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		var raw map[string]any
		if err := json.Unmarshal(line, &raw); err != nil {
			continue // tolerate corrupt lines
		}
		rec := Record{Raw: raw}
		if t, ok := raw["_type"].(string); ok {
			rec.Type = t
		}
		if ts, ok := raw["ts"].(string); ok {
			if parsed, err := time.Parse(time.RFC3339, ts); err == nil {
				rec.TS = parsed
			}
		}
		if !cutoff.IsZero() && rec.TS.Before(cutoff) {
			continue
		}
		if pid > 0 {
			v, ok := raw["pid"].(float64)
			if !ok || int(v) != pid {
				continue
			}
		}
		out = append(out, rec)
	}
	return out, sc.Err()
}
