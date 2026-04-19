# shrike — Design Spec

**Status:** Draft for review
**Date:** 2026-04-19
**Author:** christian (d56)
**Repo:** github.com/d56de/shrike (to be created)
**License:** MIT

## 1 — Summary

`shrike` is an opinionated macOS TUI tool that surfaces **suspicious processes** — runaway CPU hogs, zombies, and helper-process herds — and lets the user act on them with single keystrokes (kill, renice, sample, info). It combines the pragmatic ergonomics of [Mole](https://github.com/tw93/Mole) (one-shot interactive checkbox UI) with heuristics that standard tools like `top`, `htop`, `btop`, and `bottom` do not provide.

### Taglines

- *"Hunt runaway, zombie, and herd processes on your Mac — in seconds, not scrolling."*
- *"Shrike impales zombie processes on thorns."*

### Goals

1. A Mac user, running `shrike doctor`, sees within 5 seconds exactly which processes are *probably* misbehaving and why — with no tuning required on first run.
2. They can act on the findings (kill/renice/inspect) in ≤3 keystrokes per target.
3. The tool ships as a single signed+notarized Homebrew-installable binary.

### Non-goals (explicit)

- Cross-platform support (Linux may come later; v1 is macOS-only).
- Being a `top` replacement — no continuous full-screen dashboard in v0.1 (that's `watch`, v0.2).
- Plugin architecture — core detectors are in-tree Go.
- Generic process management (spawn, redirect, supervise).

## 2 — Architecture

### Repo layout

```
shrike/
├── cmd/
│   └── shrike/
│       └── main.go                  # Cobra root, wires subcommands
├── internal/
│   ├── core/                        # Pure engine, no TUI deps
│   │   ├── process.go               # ProcessInfo type
│   │   ├── finding.go               # Finding, Severity
│   │   ├── detector.go              # Detector interface
│   │   ├── action.go                # Action interface
│   │   └── engine.go                # parallel Detector runner
│   ├── detectors/
│   │   ├── runaway.go
│   │   ├── zombie.go
│   │   └── herd.go
│   ├── actions/
│   │   ├── info.go
│   │   ├── sample.go
│   │   ├── kill.go
│   │   └── renice.go
│   ├── history/                     # JSONL history file
│   │   ├── writer.go
│   │   ├── reader.go
│   │   └── rotate.go
│   ├── sysinfo/                     # macOS process snapshot
│   │   └── darwin.go                # //go:build darwin
│   ├── config/
│   │   └── config.go                # TOML loader + defaults
│   └── tui/
│       ├── doctor/                  # One-shot interactive screen (v0.1)
│       ├── watch/                   # Live dashboard (stub in v0.1, full in v0.2)
│       └── style/                   # Lipgloss styles
├── docs/
│   └── superpowers/specs/           # this spec + successors
├── scripts/
│   ├── release.sh                   # goreleaser wrapper
│   └── install.sh                   # curl|bash for non-brew users
├── .github/workflows/
│   ├── test.yml
│   └── release.yml
├── .goreleaser.yml
├── go.mod
├── go.sum
├── LICENSE
├── README.md
└── CHANGELOG.md
```

### Layer rule

```
  cmd/shrike                         (Cobra wiring)
       │
       ▼
  internal/tui   ◄──  internal/config
       │
       ▼
  internal/core  ◄──  internal/detectors
       │         ◄──  internal/actions
       ▼
  internal/sysinfo  ◄──  internal/history
```

Lower layers must not import higher layers. `core` + `detectors` + `actions` are testable without a terminal. `shrike doctor --json` exercises the full pipeline without any TUI code.

### Binary size target

< 15 MB statically linked. Mole is ~12 MB; Bubble Tea + Lipgloss + Cobra + gopsutil together are ~8 MB.

## 3 — Core types

### `ProcessInfo`

```go
type ProcessInfo struct {
    PID         int
    PPID        int
    User        string
    Command     string          // basename
    FullPath    string
    Args        []string
    CPUPercent  float64         // 0.0 – (100 * numCores)
    MemPercent  float64
    RSS         uint64          // bytes
    VSZ         uint64
    StartedAt   time.Time
    ElapsedTime time.Duration
    State       ProcessState
    Nice        int
}
```

Collected by `sysinfo.Snapshot(ctx)` via `libproc` bindings (`gopsutil`) with an `exec.Command("ps", ...)` fallback.

### `Finding`

```go
type Finding struct {
    Process  ProcessInfo
    Detector string              // "runaway" | "zombie" | "herd"
    Severity Severity            // Low | Medium | High | Critical
    Score    float64             // internal, used for sorting
    Reason   string              // human-readable one-liner
    Group    *HerdGroup          // non-nil only for herd findings
}
```

Findings contain a full `ProcessInfo` copy so the UI can render entries after the process has been killed.

### `Detector` interface

```go
type Detector interface {
    Name() string                           // "runaway"
    Emoji() string                          // "🔥"
    Detect(snap []ProcessInfo,
           cfg DetectorConfig) []Finding
}
```

### `Action` interface

```go
type Action interface {
    Key() rune                               // 'k'
    Name() string                            // "kill"
    Confirm() string                         // dialog text
    Destructive() bool
    Execute(ctx context.Context,
            targets []ProcessInfo) []ActionResult
}

type ActionResult struct {
    PID     int
    Err     error
    Message string                           // "sent SIGTERM"
}
```

### Engine

`core.Engine.Run(ctx)` takes a snapshot, fan-outs detectors via `errgroup`, merges findings, sorts by (Severity desc, Score desc).

## 4 — Detectors (MVP v0.1)

### 4.1 Runaway (`🔥`)

Finds processes with sustained high CPU.

**Algorithm** (`DetectorConfig` is populated from the `[runaway]` TOML section; same pattern for zombie/herd):
```
when CPU% >= CPUThreshold  (default 50.0)
 and ElapsedTime >= MinAge  (default 1h)
 and Process.Command not in Ignore  (default: WindowServer, coreaudiod, mds, mdworker)
emit Finding with
    Score    = CPU% * log10(ElapsedHours + 10)
    Severity = Score >= 200 → Critical
               Score >= 100 → High
               Score >=  50 → Medium
               else         → Low
    Reason   = "{CPU}% CPU for {ElapsedTime}"
```

### 4.2 Zombie (`🧟`)

Finds processes in state `Z` (zombie) or `T` (stopped).

**Algorithm:**
```
when State in (Zombie, Stopped)
 and ElapsedTime >= MinAge  (default 5m)
emit Finding with
    Score    = ElapsedHours * 10
    Severity = State == Zombie → High
               State == Stopped → Medium
    Reason   = "Zombie process (parent PID {PPID})" | "Stopped process"
```

Zombies cannot be killed directly. The `kill` action must detect this and suggest killing the parent with a visible hint.

### 4.3 Herd (`👥`)

Groups multiple instances of the same binary into one aggregated finding.

**Algorithm:**
```
group_by = FullPath (fallback: Command)
for each group:
    if len(group) >= HerdSize  (default 5)
     and summed_CPU >= GroupCPUThreshold  (default 30.0)
    emit Finding with
        Group    = HerdGroup{Parent: highest-RSS child, Children: rest}
        Score    = TotalCPU + len(children) * 2
        Severity = scaled like Runaway, on TotalCPU
        Reason   = "{n} {Command} processes using {TotalCPU}% CPU, {TotalRSS} MB"
```

The UI renders a herd as a single expandable row. Kill on a herd row confirms "Kill all n?". Kill on an expanded child targets just that child.

### 4.4 Parallelism

All detectors run in parallel via `errgroup`. One detector's panic does not abort the run (see §8 Error Handling).

### 4.5 v0.2 detectors (out of scope for this spec)

- **Memory Drift** — reads history JSONL, compares RSS over last 24h per PID.
- **Orphan** — PPID=1 + long-running.
- **Known Bad Actors** — user-editable list via config.

## 5 — Actions (MVP v0.1)

| Key | Action | Destructive | Description |
|-----|--------|:-----------:|-------------|
| `i` | info | no | Shows full `ProcessInfo` + lazy-loaded `lsof` counts in a modal |
| `s` | sample | no | Runs `sample <pid> 5` in background, parses top-3 call stacks, displays in modal |
| `k` | kill | yes | SIGTERM, escalates to SIGKILL after 3s if process still alive |
| `K` | kill (immediate) | yes | SIGKILL directly, bypasses TERM |
| `r` | renice | yes | `renice +10 <pid>` |

Destructive actions show a confirm dialog listing all targets before execution. Results display per-PID success/failure after execution.

### Selection scoping

- If ≥1 entries are selected with `[Space]`, actions apply to the selection.
- Otherwise, actions apply to the currently highlighted row.

## 6 — UX / TUI

### 6.1 Main doctor screen

Three-line entry format:

```
┌─ shrike doctor ──────────────────── macOS · 3 detectors · 47 procs scanned ─┐
│                                                                              │
│                          Suspicious Processes                                │
│  ═══════════════════════════════════════════════════════════════════════     │
│                                                                              │
│  ▶ ☑ 🔥 Chrome Helper          PID 93187   99.1% CPU · 23h 38m     High     │
│       /Users/christian/…/Google Chrome Helper                                │
│       Runaway — "99% CPU for 23h 38m"                                        │
│                                                                              │
│    ☑ 🔥 Virtualization.xpc     PID 13940   82.1% CPU · 4d 11h      High     │
│       /System/Library/…/com.apple.Virtualization.VirtualMachine              │
│                                                                              │
│    ☐ 👥 Figma Beta Helper ×5   combined    24.3% CPU · 0.4 GB      Medium   │
│       /Users/christian/…/Figma Beta Helper.app · [→ expand]                  │
│                                                                              │
│    ☐ 🧟 bash (zombie)          PID 47219   stopped · 2h 14m        Medium   │
│       parent PID 1 (reparented) — kill parent to reap                        │
│                                                                              │
│  ────────────────────────────────────────────────────────────────            │
│   2 selected · estimated recovery: 181% CPU freed · 1.1 GB RSS freed         │
│                                                                              │
└─ [space] select · [→] expand · [i]nfo · [s]ample · [k]ill · [r]enice · [q] ─┘
```

- **Score numbers are NOT displayed** in the main view (internal sort key only).
- Third line (Reason) is shown only on the active row.
- Summary line totals the CPU/RSS impact of selected entries.

### 6.2 Expanded herd view

Pressing `[→]` on a herd row reveals children. Each child is independently selectable. Pressing Space on the group header selects all children.

### 6.3 Confirm dialog

Destructive actions always go through a confirm dialog listing all targets, the method (SIGTERM→SIGKILL vs. SIGKILL), and an escape hatch for direct SIGKILL on `[Shift+K]`.

### 6.4 Info modal (`[i]`)

Shows: Command, Args, Parent, Children, User, Started-at, State, Nice, CPU%, RSS, VSZ, open-files count, network-connections count (via `lsof -p`, loaded lazily).

### 6.5 Sample modal (`[s]`)

Runs `sample <pid> 5` (5-second capture) in a background goroutine, parses the output, displays the top-3 hottest call stacks with percentages. If `shrike` has a matching heuristic for the observed stack pattern (e.g. V8 JS compilation loop, render deadlock), it adds a one-line interpretation. `[o]` opens the raw sample file in `$EDITOR`.

### 6.6 Keybindings

| Key              | Action                         |
|------------------|--------------------------------|
| `↑/↓` `j/k`      | Navigate                       |
| `Space`          | Toggle selection               |
| `→`              | Expand herd                    |
| `←`              | Collapse herd                  |
| `i`              | Info modal                     |
| `s`              | Sample (5s)                    |
| `k`              | Kill (TERM → KILL)             |
| `K`              | Kill immediately (SIGKILL)     |
| `r`              | Renice to +10                  |
| `?`              | Help                           |
| `q` `Esc`        | Quit / close modal             |

`doctor` is one-shot — there is no refresh key. For live updates, use `shrike watch` (v0.2).

## 7 — CLI surface

### v0.1 subcommands

```
shrike                          # alias for `shrike doctor`
shrike doctor                   # interactive TUI
shrike doctor --json            # headline JSON output on stdout, no TUI
shrike doctor --only runaway,herd  # run only the named detector(s), comma-separated
shrike log                      # display history file
shrike log --since 24h
shrike log --pid 93187
shrike config                   # print current effective config + path
shrike config edit              # open config.toml in $EDITOR
shrike version
shrike --help                   # every subcommand has its own help
```

### v0.2 additions

```
shrike watch                    # live dashboard
shrike doctor --auto            # policy-driven autonomous kill, default dry-run
```

### `doctor --json` format

One JSON object per finding on stdout:

```json
{
  "ts": "2026-04-19T08:53:12Z",
  "detector": "runaway",
  "severity": "high",
  "score": 151.2,
  "reason": "99% CPU for 23h 38m",
  "process": {
    "pid": 93187,
    "command": "Google Chrome Helper",
    "fullPath": "/Users/christian/Applications/…",
    "user": "christian",
    "cpuPercent": 99.1,
    "memPercent": 0.8,
    "rss": 148897792,
    "elapsedSeconds": 85090,
    "state": "running"
  }
}
```

Exit codes: `0` no findings, `1` findings present, `2` error.

## 8 — History (append-only JSONL)

### Location

```
$XDG_STATE_HOME/shrike/history.jsonl      # if XDG_STATE_HOME is set
~/Library/Application Support/shrike/history.jsonl   # macOS fallback
```

### Format

Append-only JSONL. Each `doctor`/`watch` run writes one `run` meta-line, then one `finding` line per finding:

```jsonl
{"_type":"run","ts":"2026-04-19T08:53:12Z","mode":"doctor","procs_scanned":47,"duration_ms":234}
{"_type":"finding","ts":"2026-04-19T08:53:12Z","detector":"runaway","score":151.2,"pid":93187,"command":"Google Chrome Helper","cpu":99.1,"rss":148897792,"elapsed_s":85090}
```

### Rotation

- On startup, if `history.jsonl` ≥ 10 MB, rename to `.1`, shift `.1 → .2`, max 3 rotations, oldest deleted.
- Configurable via `config.toml`: `[history]` `max_size_mb`, `max_rotations`.

### Privacy

Contains PIDs, command basename, full path, CPU/RSS/elapsed. No env vars, no command args, no network data. Rotation + 10 MB ceiling caps exposure.

### Reader

`history.Reader` streams lines, tolerates corrupt lines (skip + warn to `shrike.log`). v0.2 Memory-Drift detector groups readings by PID, computes RSS growth rate, emits finding if above threshold.

## 9 — Configuration

### Location

```
$XDG_CONFIG_HOME/shrike/config.toml      # if XDG_CONFIG_HOME is set
~/.config/shrike/config.toml             # fallback
```

### Default (auto-generated on first run, with comments)

```toml
[general]
default_mode = "doctor"          # or "watch"

[runaway]
cpu_threshold = 50.0
min_age = "1h"
ignore = ["WindowServer", "coreaudiod", "mds", "mdworker"]

[zombie]
min_age = "5m"

[herd]
min_size = 5
total_cpu_threshold = 30.0
known_bad_actors = []

[history]
enabled = true
max_size_mb = 10
max_rotations = 3

[ui]
severity_high_color   = "#ff5555"
severity_medium_color = "#ffa500"
severity_low_color    = "#ffd700"

# v0.2 only:
# [auto]
# kill_if_severity = "critical"
# kill_if_score_above = 200.0
# dry_run = true
```

## 10 — Error handling

### Classes

1. **Snapshot failure** — fatal. Error to stderr, exit 2.
2. **Detector failure** — non-fatal. Other detectors continue. Logged + shown as `⚠` in UI footer.
3. **Action failure** — per-target `ActionResult`. Result modal shows per-PID outcome.
4. **TUI panic** — `defer recover()` in main loop. Restore terminal, log panic stack to `crash.log`, stderr message with pointer to log + issue URL, exit 3.

### Logging

- `log/slog` structured logging to `$XDG_STATE_HOME/shrike/shrike.log` (macOS fallback `~/Library/Application Support/shrike/shrike.log`, same directory as `history.jsonl`).
- Default level `WARN`. `--log-level debug` flag for development.
- Rotated at 5 MB.
- Never written to stderr during TUI runs (would corrupt the UI).

## 11 — Testing

### Coverage targets

- `internal/core`: 90%+
- `internal/detectors`: 90%+
- `internal/actions`: 80%+ (mocked Killer interface)
- `internal/history`: 90%+
- `internal/sysinfo`: 50% (macOS-dependent)
- `internal/tui`: 40% (teatest is fragile)
- **Overall: 75%+**

### Unit tests

Per detector: happy path, filter cases, edge cases (etime=0, empty input), ignore-list respected. Per action: Kill with mock `Killer` interface, SIGTERM→SIGKILL escalation logic, renice permission failure handling.

### Integration test

`TestEngine_FullRun` loads `testdata/snapshot_busy_system.json` (captured real data, anonymized), runs the full engine, asserts specific expected findings.

### E2E (build tag `e2e`)

- `shrike doctor --json` against the real system, asserts JSON validity + exit code 0 or 1.
- `shrike version` returns something sensible.

### TUI tests

`teatest` for: initial render, `[Space]` toggles selection, `[k]` opens confirm. No deeper TUI-level assertions.

### What we do not test

- Real `kill` against real processes.
- `sample` output parsing against live data (only against fixtures).
- `libproc` bindings themselves (upstream library's responsibility).

## 12 — Release & distribution

### Pipeline

GitHub Actions release workflow triggered on `v*` tags:

1. Run full test suite.
2. `goreleaser release --clean` builds darwin/amd64 + darwin/arm64.
3. Sign + notarize via Apple Developer credentials in GH Secrets.
4. Upload release artifacts + SHA256 checksums to GitHub Release.
5. Open PR against `d56/homebrew-tap` updating the formula.

### CI

On every push/PR: `go vet`, `go test -race -coverprofile`, `golangci-lint run`. Optional Codecov badge.

### Distribution channels

- **Primary**: `brew install d56de/tap/shrike`
- **Go users**: `go install github.com/d56de/shrike/cmd/shrike@latest`
- **Curious testers**: `curl -fsSL <install-script-url> | bash`
- **v0.2**: Raycast script-commands (`d56/shrike-raycast` or in-tree `scripts/raycast/`)

### Code signing

Apple Developer account ($99/yr) is a prerequisite for notarized binaries. v0.1 may ship unsigned with a README note about System Settings override. To be decided before first release.

### Release cadence

- **v0.1.0** — MVP: `doctor` + 3 detectors + 4 actions + history + log + config.
- **v0.2.0** — `watch`, Drift detector, `--auto` policy, Raycast integration.
- **v0.3.0** — Known-Bad-Actors curated list, Orphan detector.
- **v1.0.0** — API stability guarantees on CLI flags + config schema.

### README required elements

1. Demo GIF (recorded with `vhs`).
2. One-line pitch.
3. Install block (Homebrew primary).
4. Feature list.
5. "Why not btop/htop?" comparison.
6. Link to Mole.
7. License + Contributing link.

## 13 — Open items (pre-implementation)

- [ ] Register `d56` on GitHub (currently 404) before first push.
- [ ] Decide signing strategy for v0.1 (unsigned with override vs. buy Dev account upfront).
- [ ] Decide whether to acquire `shrike.sh` domain (nice-to-have, not blocking).
- [ ] Capture a "busy system" process snapshot for `testdata/snapshot_busy_system.json`.

---

**End of spec.**
