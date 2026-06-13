# Design: `shrike watch` LaunchAgent (C2)

- **Date:** 2026-06-13
- **Status:** Approved (brainstorm complete)
- **Feature:** C2 of the four-feature roadmap — the final item. Builds on C1 (`shrike watch`).
- **Scope:** A single spec → plan → implement cycle: install/uninstall/status flags on `watch`
  plus a small `internal/agent` package that manages a macOS LaunchAgent.

## Summary

Let `shrike watch` register itself as a per-user macOS **LaunchAgent** so the watch loop runs in
the background and starts at login. Three new mutually-exclusive flags on the existing `watch`
command — `--install`, `--uninstall`, `--status` — manage the agent; with no flag, `watch`
still runs the foreground loop unchanged. A new `internal/agent` package owns plist generation
(a pure function) and the `launchctl` lifecycle (behind an injected runner, so it's testable
without touching the real system).

## Goals

- Reuse the existing `shrike watch` loop verbatim — the agent just runs `shrike watch --quiet`.
- Plist generation is a pure, unit-testable function; `launchctl` invocation is injected.
- No real `launchctl` / no real install in automated tests — only the injected fake + the
  read-only `--status`.
- Modern `launchctl` (`bootstrap`/`bootout`), idempotent install.
- Work around the launchd minimal-`PATH` gotcha so `terminal-notifier` is still found.

## Non-Goals

- No system-wide (`/Library/LaunchDaemons`, root) agent — per-user only.
- No new configuration of the loop here — the agent runs `shrike watch`, which reads `[watch]`
  config as usual.
- No cross-platform support (shrike is macOS-only; the plist path is macOS).

## Design

### 1. Flags on `watch` (`cmd/shrike/watch.go`)

Add `--install` / `--uninstall` / `--status` (bool). They are mutually exclusive (setting more
than one → a startup error). When any is set, `watch` performs the action and exits; otherwise
the existing foreground loop runs unchanged.

The management branch (before the config-load/loop) resolves the binary path via
`os.Executable()` and the log path via `history.LogPath()`, constructs an `agent.Manager`, and
dispatches:
- `--install` → `mgr.Install()` → prints `"✓ shrike watch installed as a LaunchAgent — starts at login.\n  Logs: <logpath>"`.
- `--uninstall` → `mgr.Uninstall()` → prints `"✓ shrike watch LaunchAgent removed"`.
- `--status` → `mgr.Loaded()` → prints `"running (LaunchAgent loaded)\n  plist: <path>"` or
  `"not installed\n  (run `shrike watch --install` to run in the background)"`.

### 2. `internal/agent` package

```go
const Label = "de.d56.shrike.watch" // reverse-DNS of the d56.de domain

// Plist generates the LaunchAgent plist XML. Pure.
func Plist(label, execPath, logPath string) string

// Manager manages the LaunchAgent lifecycle via launchctl.
type Manager struct {
	Label     string
	PlistPath string // ~/Library/LaunchAgents/<label>.plist
	ExecPath  string // absolute path to the shrike binary
	LogPath   string // StandardOut/ErrPath target
	UID       int
	run       func(args ...string) error // launchctl; injected (default discards output)
}

func NewManager(execPath, logPath string) (*Manager, error) // resolves home/plistPath/UID/run
func (m *Manager) Install() error
func (m *Manager) Uninstall() error
func (m *Manager) Loaded() (bool, error)
```

- `NewManager` does not import `internal/history`; the caller passes `execPath`/`logPath`. It
  resolves `~/Library/LaunchAgents/<Label>.plist`, `os.Getuid()`, and a default `run` of
  `exec.Command("launchctl", args...).Run()` (nil stdout/stderr → launchctl output is discarded,
  so `--status` stays quiet and only the exit code is used).
- `Install`: `MkdirAll(~/Library/LaunchAgents)` → write the plist (0644) → `bootout`
  (best-effort, ignore "not loaded") → `bootstrap` → so re-installing reloads cleanly.
- `Uninstall`: `bootout` (best-effort) → remove the plist file (ignore `ErrNotExist`).
- `Loaded`: `run("print", serviceTarget)`; `err == nil` ⇒ loaded.
- Targets: domain `gui/<uid>`; service `gui/<uid>/<Label>`. So
  `bootstrap gui/<uid> <plist>`, `bootout gui/<uid>/<Label>`, `print gui/<uid>/<Label>`.

### 3. Plist contents

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>            <string>de.d56.shrike.watch</string>
	<key>ProgramArguments</key> <array>
		<string>{execPath}</string>
		<string>watch</string>
		<string>--quiet</string>
	</array>
	<key>RunAtLoad</key>  <true/>
	<key>KeepAlive</key>  <true/>
	<key>StandardOutPath</key> <string>{logPath}</string>
	<key>StandardErrorPath</key> <string>{logPath}</string>
	<key>EnvironmentVariables</key> <dict>
		<key>PATH</key> <string>/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin</string>
	</dict>
</dict>
</plist>
```

- `--quiet`: `shrike.log` does **not** rotate (only `history.jsonl` does), so the per-scan
  status line would grow it without bound. `--quiet` keeps it to errors; notifications and
  history writes still happen.
- `RunAtLoad` + `KeepAlive`: start at login, relaunch on crash. `bootout` (uninstall/logout)
  stops it.
- `PATH` env: launchd agents inherit a minimal `PATH` (`/usr/bin:/bin:…`), so
  `exec.LookPath("terminal-notifier")` (in `/opt/homebrew/bin`) would fail and notifications
  would fall back to `osascript` (`/usr/bin`, always found). Setting `PATH` keeps
  `terminal-notifier` usable under the agent too.
- `execPath`/`logPath` are XML-escaped before substitution (paths could contain `&`/`<`).
  `os.Executable()` is used as-is (not symlink-resolved) so a Homebrew symlink path stays stable
  across upgrades.

### 4. Error handling

- Install/Uninstall surface `launchctl bootstrap`/file errors. `bootout`-before-`bootstrap` and
  `bootout`-on-uninstall are best-effort (a "not loaded" error is expected and ignored).
- `--install` + `--uninstall`/`--status` together → a clear mutual-exclusion error before any
  action.

## Testing (TDD)

- `internal/agent`:
  - `Plist` contains the label, exec path, `watch`/`--quiet` args, `RunAtLoad`/`KeepAlive`,
    the log path (twice), and the `PATH` env; XML-escapes a path containing `&`.
  - `Install` (Manager with a temp `PlistPath` + a recording `run`) writes the plist file with
    the expected content AND calls `run` with `["bootout","gui/<uid>/<label>"]` then
    `["bootstrap","gui/<uid>",<plistPath>]`.
  - `Uninstall` removes a pre-created plist and calls `bootout`.
  - `Loaded` returns true when `run` returns nil, false when it errors.
- `cmd`: `shrike watch --install --status` (and other pairs) → mutual-exclusion error; `shrike
  watch --status` (read-only, real launchctl, output discarded) prints "not installed" on a
  machine without the agent. **No automated step runs `--install`** (it would install a real
  agent); that is a user-driven action.

## Files touched

| File | Change |
|------|--------|
| `internal/agent/agent.go` (+ `_test.go`) | New `Plist` + `Manager` (install/uninstall/status) |
| `cmd/shrike/watch.go` | `--install`/`--uninstall`/`--status` flags + management branch |
| `README.md`, `CHANGELOG.md` | Document the LaunchAgent flags |

## Decisions log (from brainstorm)

1. **Command shape:** flags on `watch` (`--install`/`--uninstall`/`--status`) vs. a dedicated
   `agent` subcommand. Chosen: flags — simplest, discoverable under `watch --help`, matches the
   roadmap.
2. **Agent runs `--quiet`** (shrike.log doesn't rotate).
3. **RunAtLoad + KeepAlive true** (start at login, relaunch on crash).
4. **`PATH` env** including Homebrew so `terminal-notifier` works under launchd.
5. **Modern `bootstrap`/`bootout`** (`gui/<uid>`), idempotent install (bootout-then-bootstrap).
6. **No real install in automated tests** — injected `launchctl` runner + read-only `--status`.
7. **Label** `de.d56.shrike.watch`.
