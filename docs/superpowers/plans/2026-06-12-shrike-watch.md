# `shrike watch` Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a foreground `shrike watch` command that re-scans on an interval, appends to history, prints a status line per scan, and fires a deduped macOS notification on new/escalating findings.

**Architecture:** Three new units — `internal/notify` (sends notifications via terminal-notifier or osascript), `internal/watch` (pure decision logic: which findings warrant a notification, deduped/escalation-aware/summarized), and `cmd/shrike/watch.go` (thin ticker loop reusing the existing `buildEngine` + `engine.Run` + history-write path). Plus a shared `detectors.Emoji` helper and a `[watch]` config section.

**Tech Stack:** Go 1.26, cobra, standard `go test`, `os/exec` (terminal-notifier/osascript).

**Spec:** `docs/superpowers/specs/2026-06-12-shrike-watch-design.md`

**Conventions for every commit:** run `gofmt`/`goimports`, then `go test ./...`. Commit messages use `feat:` / `test:` / `docs:`. No `Co-Authored-By` trailer (attribution disabled globally for this repo).

---

## Task 1: Shared `detectors.Emoji` helper

**Files:**
- Create: `internal/detectors/emoji.go`
- Test: `internal/detectors/emoji_test.go`
- Modify: `cmd/shrike/log.go` (delegate to the shared helper, fixing the missing memleak glyph)

- [ ] **Step 1: Write the failing test**

Create `internal/detectors/emoji_test.go`:

```go
package detectors

import "testing"

func TestEmoji(t *testing.T) {
	cases := map[string]string{
		"runaway": "🔥",
		"zombie":  "🧟",
		"herd":    "👥",
		"memleak": "🧠",
		"nope":    "•",
	}
	for name, want := range cases {
		if got := Emoji(name); got != want {
			t.Errorf("Emoji(%q) = %q, want %q", name, got, want)
		}
	}
}
```

- [ ] **Step 2: Run it, verify it fails (undefined: Emoji)**

`go test ./internal/detectors/ -run TestEmoji -v`

- [ ] **Step 3: Create `internal/detectors/emoji.go`**

```go
package detectors

// Emoji returns the glyph for a detector name, matching each detector's Emoji()
// method, or "•" for an unknown name.
func Emoji(name string) string {
	switch name {
	case "runaway":
		return "🔥"
	case "zombie":
		return "🧟"
	case "herd":
		return "👥"
	case "memleak":
		return "🧠"
	default:
		return "•"
	}
}
```

- [ ] **Step 4: Delegate `cmd/shrike/log.go`'s `detectorEmoji` to it**

In `cmd/shrike/log.go`, replace the whole `detectorEmoji` function:

```go
func detectorEmoji(name any) string {
	switch name {
	case "runaway":
		return "🔥"
	case "zombie":
		return "🧟"
	case "herd":
		return "👥"
	default:
		return "•"
	}
}
```

with:

```go
func detectorEmoji(name any) string {
	s, _ := name.(string)
	return detectors.Emoji(s)
}
```

Add the import to `cmd/shrike/log.go` (in its import block):

```go
	"github.com/d56de/shrike/internal/detectors"
```

- [ ] **Step 5: Run tests + build, verify pass**

`go test ./internal/detectors/ -run TestEmoji -v && go build ./...`
Expected: PASS, build clean.

- [ ] **Step 6: Commit**

```bash
gofmt -w internal/detectors/emoji.go internal/detectors/emoji_test.go cmd/shrike/log.go
git add internal/detectors/emoji.go internal/detectors/emoji_test.go cmd/shrike/log.go
git commit -m "feat(detectors): add shared Emoji(name) helper, fix memleak glyph in log"
```

---

## Task 2: `internal/notify` — notification sender

**Files:**
- Create: `internal/notify/notify.go`
- Test: `internal/notify/notify_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/notify/notify_test.go`:

```go
package notify

import (
	"errors"
	"reflect"
	"testing"
)

type recorder struct {
	name string
	args []string
}

func newSystemWith(hasTN bool, rec *recorder) *System {
	return &System{
		lookPath: func(string) (string, error) {
			if hasTN {
				return "/opt/homebrew/bin/terminal-notifier", nil
			}
			return "", errors.New("not found")
		},
		run: func(name string, args ...string) error {
			rec.name = name
			rec.args = args
			return nil
		},
	}
}

func TestNotify_TerminalNotifier(t *testing.T) {
	var rec recorder
	s := newSystemWith(true, &rec)
	_ = s.Notify(Notification{Title: "T", Subtitle: "S", Message: "M", Group: "g"})
	if rec.name != "terminal-notifier" {
		t.Fatalf("name = %q, want terminal-notifier", rec.name)
	}
	want := []string{"-title", "T", "-message", "M", "-subtitle", "S", "-group", "g"}
	if !reflect.DeepEqual(rec.args, want) {
		t.Errorf("args = %v, want %v", rec.args, want)
	}
}

func TestNotify_TerminalNotifier_OmitsEmptyOptionals(t *testing.T) {
	var rec recorder
	s := newSystemWith(true, &rec)
	_ = s.Notify(Notification{Title: "T", Message: "M"})
	want := []string{"-title", "T", "-message", "M"}
	if !reflect.DeepEqual(rec.args, want) {
		t.Errorf("args = %v, want %v", rec.args, want)
	}
}

func TestNotify_OsascriptFallback(t *testing.T) {
	var rec recorder
	s := newSystemWith(false, &rec)
	_ = s.Notify(Notification{Title: "T", Subtitle: "S", Message: "M"})
	if rec.name != "osascript" {
		t.Fatalf("name = %q, want osascript", rec.name)
	}
	// Values are passed as argv after "--", never interpolated into the script.
	hasSep := false
	for _, a := range rec.args {
		if a == "--" {
			hasSep = true
		}
	}
	if !hasSep {
		t.Fatal("expected a -- separator before the values")
	}
	if got := rec.args[len(rec.args)-3:]; !reflect.DeepEqual(got, []string{"M", "T", "S"}) {
		t.Errorf("trailing argv = %v, want [M T S]", got)
	}
}

func TestNotify_OsascriptFallback_NoSubtitle(t *testing.T) {
	var rec recorder
	s := newSystemWith(false, &rec)
	_ = s.Notify(Notification{Title: "T", Message: "M"})
	if got := rec.args[len(rec.args)-2:]; !reflect.DeepEqual(got, []string{"M", "T"}) {
		t.Errorf("trailing argv = %v, want [M T]", got)
	}
}
```

- [ ] **Step 2: Run tests, verify they fail (undefined: System / Notification)**

`go test ./internal/notify/ -v`

- [ ] **Step 3: Create `internal/notify/notify.go`**

```go
// Package notify sends desktop notifications on macOS.
package notify

import "os/exec"

// Notification is one OS notification.
type Notification struct {
	Title    string
	Subtitle string // optional
	Message  string
	Group    string // terminal-notifier -group (replaces prior in the group); ignored by osascript
}

// Notifier sends notifications.
type Notifier interface {
	Notify(n Notification) error
}

// System is the production Notifier: terminal-notifier if on PATH, else osascript.
type System struct {
	lookPath func(string) (string, error)            // injected; default exec.LookPath
	run      func(name string, args ...string) error // injected; default exec.Command(...).Run
}

// NewSystem returns a System backed by exec.LookPath and exec.Command.
func NewSystem() *System {
	return &System{
		lookPath: exec.LookPath,
		run: func(name string, args ...string) error {
			return exec.Command(name, args...).Run() //nolint:gosec // fixed command names, values passed as args
		},
	}
}

// Notify sends n via terminal-notifier when available, else osascript.
func (s *System) Notify(n Notification) error {
	if _, err := s.lookPath("terminal-notifier"); err == nil {
		return s.run("terminal-notifier", terminalNotifierArgs(n)...)
	}
	return s.run("osascript", osascriptArgs(n)...)
}

func terminalNotifierArgs(n Notification) []string {
	args := []string{"-title", n.Title, "-message", n.Message}
	if n.Subtitle != "" {
		args = append(args, "-subtitle", n.Subtitle)
	}
	if n.Group != "" {
		args = append(args, "-group", n.Group)
	}
	return args
}

// osascriptArgs builds an AppleScript that reads its values from argv, so the
// Title/Message/Subtitle are never interpolated into the script text (no
// quoting or injection issues). ASCII "> 2" means "3 or more args".
func osascriptArgs(n Notification) []string {
	args := []string{
		"-e", "on run argv",
		"-e", "if (count of argv) > 2 then",
		"-e", "display notification (item 1 of argv) with title (item 2 of argv) subtitle (item 3 of argv)",
		"-e", "else",
		"-e", "display notification (item 1 of argv) with title (item 2 of argv)",
		"-e", "end if",
		"-e", "end run",
		"--", n.Message, n.Title,
	}
	if n.Subtitle != "" {
		args = append(args, n.Subtitle)
	}
	return args
}
```

- [ ] **Step 4: Run tests, verify pass**

`go test ./internal/notify/ -v` (4 tests pass)

- [ ] **Step 5: Commit**

```bash
gofmt -w internal/notify/notify.go internal/notify/notify_test.go
git add internal/notify/notify.go internal/notify/notify_test.go
git commit -m "feat(notify): macOS notifications via terminal-notifier or osascript"
```

---

## Task 3: `internal/watch` — notification decision logic

**Files:**
- Create: `internal/watch/watch.go`
- Test: `internal/watch/watch_test.go`

Depends on Task 1 (`detectors.Emoji`) and Task 2 (`notify.Notification`).

- [ ] **Step 1: Write the failing tests**

Create `internal/watch/watch_test.go`:

```go
package watch

import (
	"strings"
	"testing"

	"github.com/d56de/shrike/internal/core"
)

func find(det string, pid int, cmd string, sev core.Severity) core.Finding {
	return core.Finding{
		Detector: det,
		Severity: sev,
		Reason:   "reason",
		Process:  core.ProcessInfo{PID: pid, Command: cmd},
	}
}

func TestDecide_NewFindingAboveLevelNotifies(t *testing.T) {
	w := NewWatcher(core.SeverityHigh)
	got := w.Decide([]core.Finding{find("runaway", 1, "node", core.SeverityHigh)})
	if len(got) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(got))
	}
}

func TestDecide_BelowLevelIgnored(t *testing.T) {
	w := NewWatcher(core.SeverityHigh)
	got := w.Decide([]core.Finding{find("runaway", 1, "node", core.SeverityMedium)})
	if len(got) != 0 {
		t.Fatalf("expected no notification below level, got %d", len(got))
	}
}

func TestDecide_PersistingFindingSilentAfterFirst(t *testing.T) {
	w := NewWatcher(core.SeverityHigh)
	f := find("runaway", 1, "node", core.SeverityHigh)
	if got := w.Decide([]core.Finding{f}); len(got) != 1 {
		t.Fatalf("first scan should notify, got %d", len(got))
	}
	if got := w.Decide([]core.Finding{f}); len(got) != 0 {
		t.Fatalf("persisting finding should be silent, got %d", len(got))
	}
}

func TestDecide_EscalationNotifies(t *testing.T) {
	w := NewWatcher(core.SeverityHigh)
	w.Decide([]core.Finding{find("memleak", 1, "vm", core.SeverityHigh)})
	got := w.Decide([]core.Finding{find("memleak", 1, "vm", core.SeverityCritical)})
	if len(got) != 1 {
		t.Fatalf("escalation should notify, got %d", len(got))
	}
}

func TestDecide_DisappearThenReturnNotifiesAgain(t *testing.T) {
	w := NewWatcher(core.SeverityHigh)
	f := find("runaway", 1, "node", core.SeverityHigh)
	w.Decide([]core.Finding{f})
	w.Decide(nil) // gone this scan
	got := w.Decide([]core.Finding{f})
	if len(got) != 1 {
		t.Fatalf("returning finding should notify again, got %d", len(got))
	}
}

func TestDecide_MultipleFreshSummarized(t *testing.T) {
	w := NewWatcher(core.SeverityHigh)
	got := w.Decide([]core.Finding{
		find("runaway", 1, "node", core.SeverityHigh),
		find("memleak", 2, "vm", core.SeverityHigh),
	})
	if len(got) != 1 {
		t.Fatalf("expected a single summary notification, got %d", len(got))
	}
	if got[0].Title != "Shrike: 2 new issues" {
		t.Errorf("title = %q, want 'Shrike: 2 new issues'", got[0].Title)
	}
}

func TestDecide_SingleFreshDetailed(t *testing.T) {
	w := NewWatcher(core.SeverityHigh)
	got := w.Decide([]core.Finding{find("runaway", 7, "node", core.SeverityHigh)})
	if len(got) != 1 {
		t.Fatal("expected 1 notification")
	}
	if got[0].Title != "Shrike: High runaway" {
		t.Errorf("title = %q, want 'Shrike: High runaway'", got[0].Title)
	}
	if !strings.Contains(got[0].Message, "🔥") || !strings.Contains(got[0].Message, "node") {
		t.Errorf("message = %q, want emoji + command", got[0].Message)
	}
}
```

- [ ] **Step 2: Run tests, verify they fail (undefined: NewWatcher)**

`go test ./internal/watch/ -v`

- [ ] **Step 3: Create `internal/watch/watch.go`**

```go
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
```

- [ ] **Step 4: Run tests, verify pass**

`go test ./internal/watch/ -v` (7 tests pass)

- [ ] **Step 5: Commit**

```bash
gofmt -w internal/watch/watch.go internal/watch/watch_test.go
git add internal/watch/watch.go internal/watch/watch_test.go
git commit -m "feat(watch): deduped notification decision logic"
```

---

## Task 4: `WatchConfig` + defaults

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/defaults.go`
- Test: `internal/config/config_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/config/config_test.go`:

```go
func TestDefaultConfig_HasWatchDefaults(t *testing.T) {
	c := DefaultConfig()
	if time.Duration(c.Watch.Interval) != 60*time.Second {
		t.Errorf("expected interval 60s, got %v", time.Duration(c.Watch.Interval))
	}
	if c.Watch.NotifyLevel != "high" {
		t.Errorf("expected notify_level high, got %q", c.Watch.NotifyLevel)
	}
}
```

- [ ] **Step 2: Run it, verify it fails (c.Watch undefined)**

`go test ./internal/config/ -run TestDefaultConfig_HasWatchDefaults -v`

- [ ] **Step 3: Add the field + type in `internal/config/config.go`**

In the `Config` struct, the current `UI` line is the last field:

```go
	UI      UIConfig      `toml:"ui"`
}
```

Replace with:

```go
	UI      UIConfig      `toml:"ui"`
	Watch   WatchConfig   `toml:"watch"`
}
```

Then add the new type immediately after the `UIConfig` struct definition:

```go
// WatchConfig configures the `shrike watch` loop.
type WatchConfig struct {
	Interval    Duration `toml:"interval"`
	NotifyLevel string   `toml:"notify_level"`
}
```

- [ ] **Step 4: Add the default in `internal/config/defaults.go`**

The current default block ends with the `UI` default before the closing `}` of the returned `Config{...}`:

```go
		UI: UIConfig{
			SeverityHighColor:   "#ff5555",
			SeverityMediumColor: "#ffa500",
			SeverityLowColor:    "#ffd700",
			AutoRefreshInterval: 0,
		},
	}
}
```

Insert the `Watch` default after the `UI` block:

```go
		UI: UIConfig{
			SeverityHighColor:   "#ff5555",
			SeverityMediumColor: "#ffa500",
			SeverityLowColor:    "#ffd700",
			AutoRefreshInterval: 0,
		},
		Watch: WatchConfig{
			Interval:    Duration(60 * time.Second),
			NotifyLevel: "high",
		},
	}
}
```

(If the `UI` default block's fields differ slightly from the above, keep them as-is and only add the `Watch:` block after it.)

- [ ] **Step 5: Run tests, verify pass**

`go test ./internal/config/ -v`
Expected: PASS (new test + all existing).

- [ ] **Step 6: Commit**

```bash
gofmt -w internal/config/config.go internal/config/defaults.go internal/config/config_test.go
git add internal/config/config.go internal/config/defaults.go internal/config/config_test.go
git commit -m "feat(config): add [watch] config section + defaults"
```

---

## Task 5: `shrike watch` command

**Files:**
- Create: `cmd/shrike/watch.go`
- Test: `cmd/shrike/watch_test.go`
- Modify: `cmd/shrike/main.go` (register `watchCmd`)

Reuses `buildEngine` (in `doctor.go`), `history`, `notify`, `watch`, and `detectors.Emoji`.

- [ ] **Step 1: Write the failing tests**

Create `cmd/shrike/watch_test.go`:

```go
package main

import (
	"strings"
	"testing"

	"github.com/d56de/shrike/internal/core"
)

func TestParseLevel(t *testing.T) {
	if l, err := parseLevel("critical"); err != nil || l != core.SeverityCritical {
		t.Errorf("critical: got (%v, %v)", l, err)
	}
	if l, err := parseLevel("HIGH"); err != nil || l != core.SeverityHigh {
		t.Errorf("HIGH: got (%v, %v)", l, err)
	}
	if _, err := parseLevel("bogus"); err == nil {
		t.Error("expected error for bogus level")
	}
}

func TestWatchLine(t *testing.T) {
	if got := watchLine(nil); !strings.Contains(got, "clean") {
		t.Errorf("empty scan line = %q, want 'clean'", got)
	}
	line := watchLine([]core.Finding{
		{Detector: "runaway", Process: core.ProcessInfo{Command: "node"}},
	})
	if !strings.Contains(line, "🔥") || !strings.Contains(line, "node") {
		t.Errorf("line = %q, want emoji + command", line)
	}
}
```

- [ ] **Step 2: Run tests, verify they fail (undefined: parseLevel / watchLine)**

`go test ./cmd/shrike/ -run 'TestParseLevel|TestWatchLine' -v`

- [ ] **Step 3: Create `cmd/shrike/watch.go`**

```go
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	cfg "github.com/d56de/shrike/internal/config"
	"github.com/d56de/shrike/internal/core"
	"github.com/d56de/shrike/internal/detectors"
	"github.com/d56de/shrike/internal/history"
	"github.com/d56de/shrike/internal/notify"
	"github.com/d56de/shrike/internal/watch"
	"github.com/spf13/cobra"
)

var (
	watchInterval time.Duration
	watchLevel    string
	watchQuiet    bool
)

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Continuously scan and notify on new suspicious processes",
	Long: `Run shrike in the foreground, re-scanning every interval and sending a
macOS notification when a new (or escalating) finding appears at or above the
notify level. Each scan is appended to the history file. Press Ctrl-C to stop.

Examples:
  shrike watch                       # scan every 60s, notify on high+critical
  shrike watch --interval 30s        # faster cadence
  shrike watch --notify-level critical
  shrike watch --quiet               # only print when notifying`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		c, err := cfg.Load()
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}

		interval := time.Duration(c.Watch.Interval)
		if cmd.Flags().Changed("interval") {
			interval = watchInterval
		}
		if interval <= 0 {
			return fmt.Errorf("--interval must be > 0, got %v", interval)
		}

		levelStr := c.Watch.NotifyLevel
		if cmd.Flags().Changed("notify-level") {
			levelStr = watchLevel
		}
		level, err := parseLevel(levelStr)
		if err != nil {
			return err
		}

		engine := buildEngine(c, nil)
		w := watch.NewWatcher(level)
		notifier := notify.NewSystem()
		out := cmd.OutOrStdout()

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		scan := func() {
			findings, err := engine.Run(ctx)
			if err != nil {
				_, _ = fmt.Fprintf(os.Stderr, "shrike watch: scan error: %v\n", err)
				return
			}
			writeWatchHistory(c, findings)
			if !watchQuiet {
				_, _ = fmt.Fprintln(out, watchLine(findings))
			}
			for _, note := range w.Decide(findings) {
				if err := notifier.Notify(note); err != nil {
					_, _ = fmt.Fprintf(os.Stderr, "shrike watch: notify error: %v\n", err)
				}
			}
		}

		scan() // immediate first scan
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				_, _ = fmt.Fprintln(out, "shrike watch stopped")
				return nil
			case <-ticker.C:
				scan()
			}
		}
	},
}

// parseLevel maps a config/flag string to a core.Severity.
func parseLevel(s string) (core.Severity, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "low":
		return core.SeverityLow, nil
	case "medium":
		return core.SeverityMedium, nil
	case "high":
		return core.SeverityHigh, nil
	case "critical":
		return core.SeverityCritical, nil
	default:
		return 0, fmt.Errorf("invalid notify level %q (want low|medium|high|critical)", s)
	}
}

// watchLine formats the one-line per-scan status output.
func watchLine(findings []core.Finding) string {
	ts := time.Now().Format("15:04:05")
	if len(findings) == 0 {
		return ts + "  ✓ clean"
	}
	word := "findings"
	if len(findings) == 1 {
		word = "finding"
	}
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "%s  %d %s", ts, len(findings), word)
	for _, f := range findings {
		_, _ = fmt.Fprintf(&b, "  %s %s", detectors.Emoji(f.Detector), f.Process.Command)
	}
	return b.String()
}

// writeWatchHistory appends one watch run to the history file, best-effort,
// reusing the same rotate+append path as the doctor TUI rescan.
func writeWatchHistory(c cfg.Config, findings []core.Finding) {
	if !c.History.Enabled {
		return
	}
	_ = history.RotateIfNeeded(c.History.MaxSizeMB, c.History.MaxRotations)
	w, err := history.NewWriter()
	if err != nil {
		return
	}
	defer func() { _ = w.Close() }()
	_ = w.AppendRun(history.RunMeta{TS: time.Now(), Mode: "watch"}, findings)
}

func init() {
	watchCmd.Flags().DurationVar(&watchInterval, "interval", 60*time.Second, "scan interval (overrides config)")
	watchCmd.Flags().StringVar(&watchLevel, "notify-level", "high", "minimum severity to notify: low|medium|high|critical")
	watchCmd.Flags().BoolVar(&watchQuiet, "quiet", false, "suppress the per-scan status line")
}
```

- [ ] **Step 4: Register `watchCmd` in `cmd/shrike/main.go`**

In `main.go`'s `init()`, the current registrations are:

```go
func init() {
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(doctorCmd)
	rootCmd.AddCommand(logCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(statsCmd)
}
```

Add the watch command:

```go
func init() {
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(doctorCmd)
	rootCmd.AddCommand(logCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(statsCmd)
	rootCmd.AddCommand(watchCmd)
}
```

- [ ] **Step 5: Run the unit tests, verify pass**

`go test ./cmd/shrike/ -run 'TestParseLevel|TestWatchLine' -v` (2 tests pass)

- [ ] **Step 6: Build + smoke checks**

```
go build ./...
go run ./cmd/shrike watch --help
go run ./cmd/shrike watch --notify-level bogus
```

`watch --help` prints usage and exits 0. `watch --notify-level bogus` prints the invalid-level error and exits non-zero. (Do NOT run bare `shrike watch` — it loops until Ctrl-C.)

- [ ] **Step 7: Commit**

```bash
gofmt -w cmd/shrike/watch.go cmd/shrike/watch_test.go cmd/shrike/main.go
git add cmd/shrike/watch.go cmd/shrike/watch_test.go cmd/shrike/main.go
git commit -m "feat(cmd): add shrike watch command"
```

---

## Task 6: Documentation + final verification

**Files:**
- Modify: `README.md`, `CHANGELOG.md`

- [ ] **Step 1: README — Usage**

In `README.md`'s `## Usage` code block, add a `watch` line near the `doctor` lines:

```markdown
shrike watch                   # background-style loop: scan every 60s, notify on new findings
shrike watch --interval 30s    # faster cadence
shrike watch --notify-level critical
```

- [ ] **Step 2: README — short prose note**

Add a one-paragraph note (near Usage or in a short `## Watch` subsection) explaining the model:

```markdown
`shrike watch` keeps scanning in the foreground and sends a macOS notification the first
time a new (or escalating) finding appears at or above `--notify-level` (default `high`) —
deduped, so a persisting problem is announced once, not on every tick. It uses
`terminal-notifier` if installed, otherwise `osascript`. Each scan is written to history,
so `shrike stats` and the memleak detector's growth tracking both benefit from leaving it
running.
```

- [ ] **Step 3: CHANGELOG — Unreleased / Added**

In `CHANGELOG.md`, under `## [Unreleased]` → `### Added`, add:

```markdown
- `shrike watch`: a foreground loop that re-scans on an interval (default 60s), appends each
  scan to history, prints a per-scan status line, and sends a deduped macOS notification on
  new or escalating findings at/above `--notify-level` (default `high`). Notifies via
  `terminal-notifier` when present, else `osascript`. Configurable via the `[watch]` section.
```

- [ ] **Step 4: Commit**

```bash
git add README.md CHANGELOG.md
git commit -m "docs: document shrike watch"
```

- [ ] **Step 5: Final full verification**

```
go test ./... -race -count=1
go vet ./...
```

Expected: all packages green, no vet output.

---

## Self-Review notes (resolved during planning)

- **Spec coverage:** `internal/notify` (T2), `internal/watch` decision logic (T3), `detectors.Emoji` shared helper + log fix (T1), `WatchConfig` + defaults (T4), `cmd/shrike/watch.go` loop + signals + flags + history reuse + register (T5), docs incl. the model note (T6). All spec sections mapped.
- **No-injection notifications:** osascript values pass via argv after `--`; terminal-notifier values pass as exec args. Tested in T2 (no shell interpolation).
- **Decision logic isolation:** `internal/watch.Decide` is a pure function of (findings, dedup state) → notifications; deterministic tests (T3) feed finding sequences. The cmd loop (T5) is thin and only its pure helpers (`parseLevel`, `watchLine`) are unit-tested; the ticker/signal loop is exercised via the smoke check.
- **Type consistency:** `notify.Notification{Title,Subtitle,Message,Group}`, `notify.Notifier`, `notify.NewSystem()`, `watch.NewWatcher(core.Severity)`, `watch.Watcher.Decide([]core.Finding) []notify.Notification`, `detectors.Emoji(string)`, `cfg.WatchConfig{Interval Duration, NotifyLevel string}`, `parseLevel`, `watchLine`, `writeWatchHistory` used consistently across T1–T6.
- **Reuse:** `buildEngine(c, nil)` (doctor.go), `history.RotateIfNeeded`/`NewWriter`/`AppendRun` (runRescan pattern), `engine.Run(ctx)` — no engine/detector change.
