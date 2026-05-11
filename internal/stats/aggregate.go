// Package stats aggregates `shrike` history.jsonl into per-day rollups for
// rendering activity heatmaps and summary tables.
package stats

import (
	"sort"
	"time"

	"github.com/d56de/shrike/internal/history"
)

// Day is the rolled-up activity for one calendar day (local timezone).
type Day struct {
	Date         time.Time // midnight, local TZ
	Scans        int       // count of "run" records
	Findings     int       // count of "finding" records (any severity)
	HighFindings int       // findings with severity high or critical
}

// Summary is the headline metrics across the full window.
type Summary struct {
	From          time.Time
	To            time.Time
	TotalScans    int
	TotalFindings int
	ActiveDays    int // days with at least one scan
	LongestStreak int // longest run of consecutive active days
	CurrentStreak int // active days ending at (and including) `To`
}

// Aggregate reads history.jsonl, filters to the [from, to] window (inclusive
// on both ends, local-day granularity), and returns one Day per calendar day
// in chronological order. Days with no activity are included as zero rows so
// downstream renderers can produce a regular grid.
//
// `from` and `to` are normalized to local midnight before bucketing. An empty
// or missing history file produces a zero-activity window, not an error.
func Aggregate(from, to time.Time) ([]Day, Summary, error) {
	from = startOfDay(from)
	to = startOfDay(to)
	if to.Before(from) {
		from, to = to, from
	}

	// Read all records since `from` — Read takes a relative duration.
	since := time.Since(from)
	if since < 0 {
		since = 0
	}
	records, err := history.Read(since, 0)
	if err != nil {
		return nil, Summary{}, err
	}

	// Bucket by local YYYY-MM-DD key.
	buckets := map[string]*Day{}
	for ts := from; !ts.After(to); ts = ts.AddDate(0, 0, 1) {
		key := dayKey(ts)
		buckets[key] = &Day{Date: ts}
	}
	for _, r := range records {
		local := r.TS.Local()
		day := startOfDay(local)
		if day.Before(from) || day.After(to) {
			continue
		}
		bucket, ok := buckets[dayKey(day)]
		if !ok {
			continue
		}
		switch r.Type {
		case "run":
			bucket.Scans++
		case "finding":
			bucket.Findings++
			if sev, _ := r.Raw["severity"].(string); sev == "high" || sev == "critical" {
				bucket.HighFindings++
			}
		}
	}

	// Emit in chronological order.
	days := make([]Day, 0, len(buckets))
	for _, d := range buckets {
		days = append(days, *d)
	}
	sort.Slice(days, func(i, j int) bool { return days[i].Date.Before(days[j].Date) })

	return days, summarize(days, from, to), nil
}

// summarize computes window-wide metrics from the day buckets.
func summarize(days []Day, from, to time.Time) Summary {
	s := Summary{From: from, To: to}
	streak, best := 0, 0
	for _, d := range days {
		s.TotalScans += d.Scans
		s.TotalFindings += d.Findings
		if d.Scans > 0 {
			s.ActiveDays++
			streak++
			if streak > best {
				best = streak
			}
		} else {
			streak = 0
		}
	}
	s.LongestStreak = best
	// CurrentStreak: walk backwards from the last day while still active.
	for i := len(days) - 1; i >= 0; i-- {
		if days[i].Scans == 0 {
			break
		}
		s.CurrentStreak++
	}
	return s
}

// startOfDay returns the local midnight of t.
func startOfDay(t time.Time) time.Time {
	t = t.Local()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// dayKey formats a local date as "2006-01-02" for use as a map key.
func dayKey(t time.Time) string { return t.Format("2006-01-02") }
