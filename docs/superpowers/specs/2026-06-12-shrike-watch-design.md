# Design: `shrike watch` (C1)

- **Date:** 2026-06-12
- **Status:** Approved (brainstorm complete)
- **Feature:** C1 of the four-feature roadmap. C2 (LaunchAgent autostart) is a separate later cycle.
- **Scope:** A single spec → plan → implement cycle: a new foreground `shrike watch` command, a notification package, and a watcher decision package.

## Summary

Add `shrike watch`: a foreground loop that re-runs the detector engine on an interval,
appends each scan to the history file, prints a concise status line per scan, and fires a
**macOS notification** when a new (or escalating) finding appears at or above a severity
threshold. The key design problem — notification fatigue — is solved by a dedup model that
notifies once per distinct problem, never on every tick.

This also gives the memleak detector (feature B) its first continuous sampling source, and
keeps the history file populated so `shrike stats` (feature D) has real data.

## Goals

- Reuse the existing engine (`buildEngine` + `engine.Run`) and history-write path — no detector
  or engine change.
- No new Go dependency. Notifications shell out to `terminal-notifier` (if present) or
  `osascript` (always present on macOS).
- A low-noise notification model: notify on new/escalating findings only, deduped.
- The notification *decision* logic is a pure, unit-testable unit, isolated from time, the
  engine, and the OS.

## Non-Goals

- No LaunchAgent / background autostart (that is C2).
- No notification *actions* (click-to-open) — informational only.
- No persisted dedup state across `watch` restarts (in-memory for the loop's lifetime).
- No change to the detectors themselves.

## Architecture

Three new units with clear boundaries:

### 1. `internal/notify` — sending a notification

```go
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

func NewSystem() *System
func (s *System) Notify(n Notification) error
```

- If `lookPath("terminal-notifier")` succeeds: run
  `terminal-notifier -title <Title> -subtitle <Subtitle> -message <Message> -group <Group>`
  (empty fields omitted). Args are passed as exec args — no shell, no injection risk.
- Else: run `osascript` passing the values as **argv**, not interpolated into the script, to
  avoid any quoting/injection problem:
  ```
  osascript -e 'on run argv' \
    -e 'if (count of argv) > 2 then' \
    -e '  display notification (item 1 of argv) with title (item 2 of argv) subtitle (item 3 of argv)' \
    -e 'else' \
    -e '  display notification (item 1 of argv) with title (item 2 of argv)' \
    -e 'end if' \
    -e 'end run' \
    -- <Message> <Title> [<Subtitle>]
  ```
  (ASCII `> 2` — i.e. ≥3 args — avoids any non-ASCII operator in the script.)
- `lookPath`/`run` are injected so tests assert the exact command + args built for each branch
  without touching the OS.

### 2. `internal/watch` — the notification decision (pure logic)

```go
// Watcher decides which findings warrant a notification, deduping across scans.
type Watcher struct {
	level core.Severity            // notify threshold (inclusive)
	seen  map[string]core.Severity // dedup key -> last-notified severity
}

func NewWatcher(level core.Severity) *Watcher

// Decide updates the dedup state from the current scan's findings and returns the
// notifications to send (already filtered to >= level, deduped, escalation-aware,
// and summarized). It performs no I/O.
func (w *Watcher) Decide(findings []core.Finding) []notify.Notification
```

Decision rules (`Decide`):
- Consider only findings with `Severity >= w.level`.
- Dedup key: `"<detector>:<pid>:<command>"`.
- A finding is **fresh** (warrants notification) if its key is not in `seen`, OR its severity is
  strictly greater than the last-seen severity for that key (escalation).
- Replace `seen` with exactly this scan's `>=level` findings (key → severity). Therefore: a
  persisting finding stays in `seen` and does not re-notify; a finding that disappears is
  dropped, so if it returns later it notifies again; the **first scan** notifies for everything
  `>=level` (empty initial state).
- Summarize the fresh findings:
  - 0 fresh → no notification.
  - 1 fresh → a **detailed** notification: Title `"Shrike: <Severity> <detector>"`,
    Message `"<emoji> <command> (PID <pid>) — <reason>"`, Group `"shrike:<key>"`.
  - ≥2 fresh → one **summary** notification: Title `"Shrike: <N> new issues"`,
    Message a comma-joined `"<emoji> <command>"` list, Group `"shrike:summary"`.

`internal/watch` imports `core`, `notify`, and `detectors` (for the emoji lookup, see below).
No dependency on time, the engine, cobra, or the OS.

### 3. `cmd/shrike/watch.go` — the thin loop

- New cobra command `watch`, registered in `main.go`.
- Flags: `--interval` (Duration, default from config), `--notify-level` (string:
  low|medium|high|critical, default from config), `--quiet` (suppress the per-scan line).
- Builds the engine once via the existing `buildEngine(c, nil)` (all detectors — this is where
  memleak's growth detection shines), constructs `watch.NewWatcher(level)` and
  `notify.NewSystem()`.
- Loop, driven by `signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)` + a
  `time.Ticker`: on each tick (and once immediately at start),
  1. `findings, _ := engine.Run(ctx)`,
  2. append to history (reusing the `runRescan` pattern: `RotateIfNeeded` + `NewWriter` +
     `AppendRun` with `Mode: "watch"`), gated by the history-enabled config,
  3. unless `--quiet`, print one status line: `15:04:05  <N> findings  <emoji> <cmd> …` or
     `15:04:05  ✓ clean`,
  4. `for _, n := range watcher.Decide(findings) { _ = notifier.Notify(n) }` (notification
     failure is logged to stderr, never fatal).
- On `ctx.Done()`: print `"shrike watch stopped"`, return nil (exit 0).

### 4. Shared emoji helper — `detectors.Emoji`

The detector→emoji mapping is needed by `internal/watch` (notification text) and already exists,
incompletely, as `detectorEmoji` in `cmd/shrike/log.go` (it predates memleak and returns `•`
for it). To have one source of truth:

- Add `func Emoji(name string) string` to `internal/detectors` covering all four detectors
  (`runaway` 🔥, `zombie` 🧟, `herd` 👥, `memleak` 🧠; default `•`).
- `internal/watch` uses `detectors.Emoji`.
- `cmd/shrike/log.go`'s `detectorEmoji(name any)` delegates to it (asserting the `any` to string),
  which also fixes the pre-existing missing-memleak-emoji gap in `shrike log`.

## Config

New `[watch]` section:

```go
type WatchConfig struct {
	Interval    Duration `toml:"interval"`     // default 60s
	NotifyLevel string   `toml:"notify_level"` // default "high"
}
```

Defaults: `Interval: 60s`, `NotifyLevel: "high"`. The command flags override these per-run.
A `parseLevel(string) (core.Severity, error)` helper (in the watch command) maps the string to
`core.Severity`; an invalid value is a startup error.

## Error handling

- `engine.Run` error on a tick: print the error to stderr and continue the loop (a transient
  scan failure must not kill a long-running watcher).
- History write error: best-effort (ignored, as in `runRescan`).
- Notification send error: log to stderr, never fatal.
- Invalid `--notify-level` / `--interval` ≤ 0: validated at startup, returns an error before the
  loop.

## Testing (TDD)

- `internal/notify`:
  - terminal-notifier present → asserts the exact `terminal-notifier` argv (title/subtitle/
    message/group), via injected `lookPath`/`run`.
  - terminal-notifier absent → asserts the `osascript` argv (values passed after `--`).
  - empty optional fields omitted correctly.
- `internal/watch` (`Decide`, deterministic — feed finding sequences):
  - new finding ≥ level → one notification; below level → none.
  - persisting finding (same key, same severity) on the next scan → no notification.
  - escalation (same key, higher severity) → notification.
  - disappear then return → notification again.
  - ≥2 fresh in one scan → a single summary notification; exactly 1 → a detailed one.
  - first scan with existing findings → notifies (empty initial state).
- `internal/detectors`: `Emoji` returns the right glyph per name and `•` for unknown.
- `cmd`: `shrike watch --help` works; `--notify-level bogus` errors.

## Files touched

| File | Change |
|------|--------|
| `internal/notify/notify.go` (+ `_test.go`) | New notification sender |
| `internal/watch/watch.go` (+ `_test.go`) | New watcher decision logic |
| `internal/detectors/emoji.go` (+ `_test.go`) | New shared `Emoji(name)` helper |
| `cmd/shrike/watch.go` | New `watch` command (loop, signals, flags) |
| `cmd/shrike/main.go` | Register `watchCmd` |
| `cmd/shrike/log.go` | `detectorEmoji` delegates to `detectors.Emoji` |
| `internal/config/config.go`, `defaults.go` | `WatchConfig` + defaults |
| `README.md`, `CHANGELOG.md` | Document `shrike watch` |

## Decisions log (from brainstorm)

1. **Scope:** C1 (`shrike watch`) only; C2 (LaunchAgent) is a separate later cycle.
2. **Notification model:** notify on new/escalating findings ≥ a severity threshold, deduped by
   `detector:pid:command`; summarize ≥2 fresh into one notification; first scan notifies.
3. **Mechanism:** `terminal-notifier` if on PATH, else `osascript` (values via argv, no
   injection); zero new Go dependency.
4. **Defaults:** interval 60s, notify_level "high".
5. **Output:** one status line per scan (suppressible with `--quiet`), independent of the
   notification subset.
6. **Decision logic isolated** in `internal/watch` (pure, no I/O) for testability.
