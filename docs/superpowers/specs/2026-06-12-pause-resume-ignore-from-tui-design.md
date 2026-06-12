# Design: Pause/Resume + ignore-from-TUI

- **Date:** 2026-06-12
- **Status:** Approved (brainstorm complete)
- **Feature:** A of a four-feature roadmap (others: memleak detector, `shrike watch` + notifications, stats integration)
- **Scope:** A single spec → plan → implement cycle. Two small, self-contained additions to the existing doctor TUI.

## Summary

Add two capabilities to `shrike doctor`:

1. **Pause/Resume** — a keyed action `[p]` that freezes a running process (`SIGSTOP`) or
   resumes a stopped one (`SIGCONT`), as a context-sensitive toggle. Paused processes stay
   visible in the session so they remain resumable.
2. **ignore-from-TUI** — a key `[I]` that permanently adds the selected finding's command to
   an ignore list, persisted to a separate machine-managed `ignore.toml`, and immediately
   filters matching findings out of the current view.

Both are "softer" alternatives to kill: pause buys time without losing process state; ignore
silences false positives without hand-editing config.

## Goals

- Pause maps onto the existing `Killer` interface — no new syscall wrapper.
- A paused process is always resumable from within the same TUI session.
- Ignoring a process never corrupts the user's hand-written `config.toml`.
- Detectors remain **unchanged** — they keep reading their own `Ignore` slice.
- The `core.Action` interface remains **unchanged**.

## Non-Goals

- No PID-based or path-based ignore granularity. Ignore matches on **command basename**,
  consistent with the existing `slices.Contains(ignore, p.Command)` detector semantics.
- No cross-session persistence of paused state (paused PIDs are session-scoped).
- No global "ignore everywhere" list — ignore is scoped to the finding's own detector.
- No new config-editing UX beyond the single `[I]` hotkey.

## Design

### 1. Pause/Resume — new `core.Action`

- New file `internal/actions/pause.go` (+ `pause_test.go`), mirroring `kill.go`.
- Reuses the existing `Killer` interface (`Signal(pid, sig)`, `Alive(pid)`) for testability.
- Single context-sensitive toggle bound to `[p]`:
  - `Key() == 'p'`, `Name() == "pause"`.
  - `Destructive() == false`, `Confirm() == ""` → **no confirmation**, instant toggle.
  - `Execute` inspects each target's `State`:
    - `StateStopped` → `SIGCONT`, `ActionResult.Message == "resumed"`.
    - `StateZombie` → skip, `Message == "skipped (zombie)"` (a dead process can't be stopped).
    - otherwise → `SIGSTOP`, `Message == "paused"`.
- Registered by adding `actions_.NewPause()` to the `acts` slice in `cmd/shrike/doctor.go`.
  The existing key-dispatch in `update.go` wires it into the TUI, footer, and (skipped)
  confirm flow automatically.

### 2. Resume visibility — pinned paused PIDs in the model

Problem: a paused process drops to ~0% CPU, so the runaway detector stops flagging it on the
next scan and its row disappears — leaving no way to resume it.

Solution:

- `Model` gains `PausedPIDs map[int]struct{}`.
- The `ActionMsg` handler (`update.go`) reads each `ActionResult.Message`:
  `"paused"` → add PID; `"resumed"` → remove PID.
- List rendering always includes pinned PIDs even when no detector flags them, tagged
  `⏸ paused`. Pins clear on resume or when the TUI exits (session-scoped).

### 3. ignore-from-TUI — model-handled, `[I]`

Rationale: ignore edits **config** and needs the **detector context** of the finding
(`finding.Detector`), which the process-oriented `core.Action` interface does not carry
(`Execute` receives `[]ProcessInfo`, not `[]Finding`). So ignore is handled directly in the
model, where cursor → `Finding` → `Detector` is available. This keeps the `Action` interface
and all detectors untouched.

- Key `[I]` (Shift+i; mnemonic "Ignore", parallel to the `k`/`K` capital-variant pattern),
  handled in `handleListKey`.
- Uses the selected finding: appends `finding.Process.Command` to the section named by
  `finding.Detector` in `ignore.toml`.
- Shows a confirm dialog (persistent change + suppresses *all* processes of that name):
  > Ignore all 'node' processes in the runaway detector? (saved to ignore.toml)
- **Confirm wiring:** the existing confirm modal is bound to a `core.Action`
  (`PendingAction` + `runAction`). Ignore is model-handled, so it carries its own pending
  state — `IgnorePending *core.Finding` — and reuses `ModeConfirm` for rendering. In
  `handleConfirmKey`, `y/enter` branches: if `IgnorePending != nil`, commit the ignore;
  otherwise run the pending action. `n`/`esc` clears whichever is pending. This avoids
  forcing ignore through the process-oriented `Action` path.
- On confirm: persist the entry **and** immediately filter every matching finding
  (same detector + command) out of the current view. No rescan required.
- For `herd` findings, the ignored command is the herd parent's command
  (`finding.Process.Command`), appended to the `[herd]` section.

### 4. `ignore.toml` — new config module

- New file `internal/config/ignore.go` (+ `ignore_test.go`).
- Path `~/.config/shrike/ignore.toml`, resolved by `config.IgnorePath()` mirroring `Path()`
  (honors `$XDG_CONFIG_HOME`).
- Structured per-detector, mirroring `config.toml` section names:

  ```toml
  # Managed by shrike — entries added via [I] in `shrike doctor`.
  # Safe to edit or delete by hand.
  [runaway]
  ignore = ["node"]
  ```

- Append is **idempotent** (dedup within a section) and creates the file (with header
  comment) if absent.
- `config.Load()` merges `ignore.toml` entries into the matching detector `Ignore` slices
  after decoding `config.toml`. Detectors are unaware the entries came from a second file.

### 5. Error handling

- Pause/Resume: signal errors surface per-PID via `ActionResult.Err`/`Message`
  (e.g. "not permitted"), exactly like `kill`.
- ignore.toml write failure: surface the error in the results banner; do not filter the
  finding out of the view if the write failed (keep state consistent with disk).
- Missing/corrupt `ignore.toml` on load: treat as empty (log nothing fatal); never block
  `config.Load()`.

## Testing (TDD)

Mirrors existing test patterns (`kill_test.go`, `config_test.go`, doctor model tests).

- `pause_test.go`: fake `Killer` asserts `SIGSTOP` on a running target, `SIGCONT` on a stopped
  target, zombie skipped, permission error reported.
- `ignore_test.go`: create file with header; append to existing section; dedup; merge
  `config.toml` + `ignore.toml` into detector `Ignore` slices.
- Model tests: a paused PID stays visible after a rescan that no longer flags it; `[I]` on a
  finding writes the correct section and removes matching findings from the view.

## Files touched

| File | Change |
|------|--------|
| `internal/actions/pause.go` (+ `_test.go`) | New Pause toggle action |
| `internal/config/ignore.go` (+ `_test.go`) | New ignore.toml read/append/merge |
| `internal/config/config.go` | Merge ignore.toml in `Load()`; add `IgnorePath()` |
| `internal/tui/doctor/model.go` | `PausedPIDs`, ignore-confirm state |
| `internal/tui/doctor/update.go` | Register Pause; `[I]` handling; pin/unpin on results |
| `internal/tui/doctor/view.go` | `⏸ paused` marker; `[p]`/`[I]` in footer + help |
| `cmd/shrike/doctor.go` | Register `NewPause()`; pass IgnorePath into model |
| `README.md`, `CHANGELOG.md` | Document the two new keys |

## Decisions log (from brainstorm)

1. **Resume visibility:** pin paused PIDs in the session (vs. relying on the stopped detector,
   vs. fire-and-forget). Chosen for cleanest UX.
2. **Config write strategy:** separate generated `ignore.toml` merged at load (vs. surgical
   edit of `config.toml`, vs. full re-marshal). Chosen to never touch the user's file.
3. **Pause confirm:** none (reversible, snappy).
4. **Ignore confirm:** yes (persistent change + suppresses all by name).
5. **Keybindings:** `[p]` pause/resume toggle, `[I]` ignore.
