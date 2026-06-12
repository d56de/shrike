// Package watch decides which findings warrant a notification, deduping across
// scans so a persisting problem is announced once, not on every tick.
package watch

import (
	"fmt"
	"strings"

	"github.com/d56de/shrike/internal/core"
	"github.com/d56de/shrike/internal/detectors"
	"github.com/d56de/shrike/internal/notify"
)

// Watcher tracks which findings have already been notified.
type Watcher struct {
	level core.Severity
	seen  map[string]core.Severity // dedup key -> last-notified severity
}

// NewWatcher returns a Watcher that notifies for findings at or above level.
func NewWatcher(level core.Severity) *Watcher {
	return &Watcher{level: level, seen: map[string]core.Severity{}}
}

// Decide updates the dedup state from the current scan's findings and returns
// the notifications to send: filtered to >= level, deduped, escalation-aware,
// and summarized. It performs no I/O.
func (w *Watcher) Decide(findings []core.Finding) []notify.Notification {
	current := map[string]core.Severity{}
	var fresh []core.Finding
	for _, f := range findings {
		if f.Severity < w.level {
			continue
		}
		k := key(f)
		current[k] = f.Severity
		if prev, ok := w.seen[k]; !ok || f.Severity > prev {
			fresh = append(fresh, f)
		}
	}
	w.seen = current
	return summarize(fresh)
}

func key(f core.Finding) string {
	return fmt.Sprintf("%s:%d:%s", f.Detector, f.Process.PID, f.Process.Command)
}

func summarize(fresh []core.Finding) []notify.Notification {
	switch len(fresh) {
	case 0:
		return nil
	case 1:
		return []notify.Notification{detail(fresh[0])}
	default:
		return []notify.Notification{summary(fresh)}
	}
}

func detail(f core.Finding) notify.Notification {
	return notify.Notification{
		Title:   fmt.Sprintf("Shrike: %s %s", capitalize(f.Severity.String()), f.Detector),
		Message: fmt.Sprintf("%s %s (PID %d) — %s", detectors.Emoji(f.Detector), f.Process.Command, f.Process.PID, f.Reason),
		Group:   "shrike:" + key(f),
	}
}

func summary(fresh []core.Finding) notify.Notification {
	parts := make([]string, 0, len(fresh))
	for _, f := range fresh {
		parts = append(parts, detectors.Emoji(f.Detector)+" "+f.Process.Command)
	}
	return notify.Notification{
		Title:   fmt.Sprintf("Shrike: %d new issues", len(fresh)),
		Message: strings.Join(parts, ", "),
		Group:   "shrike:summary",
	}
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
