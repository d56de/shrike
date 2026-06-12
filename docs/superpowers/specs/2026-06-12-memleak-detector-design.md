# Design: memleak detector

- **Date:** 2026-06-12
- **Status:** Approved (brainstorm complete)
- **Feature:** B of the four-feature roadmap (A shipped; C `shrike watch`, D stats integration pending)
- **Scope:** A single spec → plan → implement cycle. One new stateful detector plus config/wiring.

## Summary

Add a fourth detector, **memleak** (🧠), to `shrike doctor`. It is a **hybrid**: it flags a
process when it is either a **memory hog** (RSS over an absolute threshold — fires on a single
scan) **or** a **leak** (RSS growing steadily across scans — confirmed once enough samples
accumulate). Growth detection escalates severity.

Unlike the other three detectors, memleak is **stateful**: it keeps a small per-PID RSS sample
history in memory and accumulates one sample per scan. The engine constructs detectors once and
reuses them across rescans, so this state persists across the TUI's rescans / auto-refresh /
`shrike watch` (feature C) without any change to the `Detector` interface or the history file.

## Goals

- Real growth signal ("this process is leaking"), not just "this process is big".
- Fits the existing `core.Detector` interface — no engine/interface change.
- Sees every process: detectors receive the full snapshot, so coverage is not limited to
  already-flagged processes.
- Fires on a one-shot `shrike doctor` / `--json` via the hog threshold (growth stays silent
  with a single sample — a trend cannot be derived from one point).
- Deterministic and unit-testable: no wall-clock dependency.

## Non-Goals

- No cross-session / cross-day leak tracking. State is in-memory and resets on restart.
  (Long-running detection is what `shrike watch` will enable later.)
- No change to `internal/history` (it stores findings only, which is the wrong source for
  whole-machine growth tracking).
- No new config knobs beyond the essentials; growth heuristics are internal constants.

## Key facts that shaped this design

1. `engine.Run` passes the **full process snapshot** to every detector's `Detect`, not just
   findings. memleak can observe any process.
2. `internal/history` records only **findings** (a leaking process that no detector has flagged
   never appears there) — so the history file cannot be the growth data source.
3. Detectors are constructed once and reused across `Run` calls; `Run`s are sequential and a
   given detector's `Detect` runs in exactly one goroutine per `Run`. A pointer-receiver
   detector can therefore safely carry state across scans with no data race.

## Design

### 1. Detector type (`internal/detectors/memleak.go`)

```go
type Memleak struct {
	hist map[int]*pidHistory // by PID; in-memory, accumulates across scans
}

type pidHistory struct {
	startedAt time.Time // from ProcessInfo.StartedAt — detects PID reuse
	rss       []uint64  // ring buffer, newest last, capped at maxSamples
}

func NewMemleak() *Memleak // returns a pointer so state persists across Detect calls
```

- `Name() string` → `"memleak"`, `Emoji() string` → `"🧠"`.
- Internal constants: `maxSamples = 10`, `minSamples = 4`, `growthMB = 100`,
  `growthPct = 0.20`, `growthFloorMB = 250`.

### 2. Detection rule (per `Detect` call)

1. Read config (with defaults): `rss_threshold` (bytes, default 1 GB = 1024 MB), `min_age`
   (default 5 min), `ignore []string`.
2. Build the set of live PIDs from the snapshot. **Evict** any tracked PID not in the set
   (process exited) — bounds memory.
3. For each process `p` in the snapshot:
   - Skip if `p.Command` is in `ignore`.
   - **Track** `p` if `p.RSS ≥ growthFloor` or it is already tracked. On track:
     - If a tracked PID's stored `startedAt != p.StartedAt`, **reset** its history (PID reuse).
     - Append `p.RSS` to the ring buffer (drop oldest beyond `maxSamples`).
   - Compute over the ring buffer (`oldest` = front, `newest` = back):
     - `growing := len(buf) ≥ minSamples && newest ≥ oldest + growthBytes && newest ≥ oldest*(1+growthPct)`
   - `hog := p.RSS ≥ rss_threshold && p.ElapsedTime ≥ min_age`
   - `leak := growing && p.RSS ≥ growthFloor`
   - If `hog || leak`, emit a `core.Finding`.

The `growthFloor` (250 MB) keeps trivial processes out of tracking and out of leak findings.
The `growthPct`+`growthMB` net-growth test over `minSamples` samples is robust to GC jitter (a
single dip does not reset the trend; only sustained net growth qualifies).

### 3. Severity & reason

```go
gb := float64(rss) / (1 << 30)
score := gb * 50
if growing {
	growthGB := float64(newest-oldest) / (1 << 30)
	score += 50 + growthGB*150
}
```

Mapping: `critical ≥ 250`, `high ≥ 120`, `medium ≥ 50`, else `low`.

Examples: 1 GB hog → 50 → medium; 2.4 GB hog (not growing) → 120 → high; 2.4 GB +600 MB
growth → ~258 → critical; 300 MB +150 MB growth → ~87 → medium.

Reason text:
- growing: `"2.4 GB RSS, +600 MB and climbing"`
- hog only: `"2.4 GB RSS"`

(RSS rendered in human units — GB with one decimal at/above 1 GB, otherwise MB; growth in MB.
No wall-clock — "and climbing" rather than "in N min", keeping the detector pure. A small local
`formatBytes` helper in the detectors package renders both, alongside the existing
`formatElapsed` helper there — no cross-package import.)

### 4. Concurrency note

`Memleak.Detect` mutates `hist` and is **not** safe for concurrent calls on the same instance.
The engine never does that: each detector's `Detect` runs once per `Run` (one goroutine), and
`Run`s are sequential. Documented on the type so a future caller does not parallelise it.

### 5. Config (`internal/config`)

New section, mirroring the other detectors:

```go
type MemleakConfig struct {
	RSSThresholdMB int      `toml:"rss_threshold_mb"`
	MinAge         Duration `toml:"min_age"`
	Ignore         []string `toml:"ignore"`
}
```

Defaults (`defaults.go`): `RSSThresholdMB: 1024`, `MinAge: 5m`, `Ignore: []string{}`.

The growth constants are **not** configurable (YAGNI) — users tune the one threshold that
matters.

### 6. Feature-A synergy: ignore-from-TUI

- `internal/config/ignore.go` `section()` gains a `"memleak"` case so `[I]` can persist a
  memleak ignore to `ignore.toml`.
- `internal/tui/doctor/update.go` `[I]` guard adds `"memleak"` to the allowed detectors.

So a user can ignore a noisy memleak finding directly from the TUI, same as the other detectors.

### 7. Wiring (`cmd/shrike/doctor.go`)

- `buildEngine`: append `detectors.NewMemleak()` to the detector slice; add it to the `--only`
  selection; add a `"memleak"` entry to the `configs` map carrying `rss_threshold` (bytes,
  converted from MB), `min_age` (time.Duration), and `ignore`.
- README/usage `--only` text gains `memleak`.

### 8. One-shot vs multi-sample behavior

- `shrike doctor` / `--json` (single `Run`): only the **hog** path can fire (1 sample → no
  growth). A leaking-but-under-threshold process is silent until the next sample.
- TUI with auto-refresh, or `shrike watch` (feature C): samples accumulate, the **leak** path
  activates, severity escalates as growth is confirmed.

This is documented in README so the one-shot limitation is not surprising.

## Error handling

- Config type assertions fall back to defaults if missing/wrong type (mirrors runaway).
- No I/O in `Detect` — it cannot fail; it returns findings only.

## Testing (TDD)

Table-driven, deterministic (no clock — feed snapshot sequences):

- **hog one-shot:** a single snapshot with RSS ≥ threshold and age ≥ min_age → one medium
  finding; below threshold → none.
- **leak escalation:** feed a monotonically growing RSS sequence (≥ minSamples snapshots) above
  growthFloor → fires from sample `minSamples` with escalating severity and a "+N MB and
  climbing" reason.
- **GC jitter:** an up/down/up oscillating sequence with no net growth → no leak finding.
- **min_age gate:** a huge-RSS but young process → no hog finding.
- **eviction:** a PID present (with a near-leak sample history), then absent for a scan, then
  re-introduced at high RSS → does **not** immediately flag as growth, because its history was
  evicted and it starts fresh with one sample. (Behavioral assertion; no internals exported.)
- **PID reuse:** same PID with a different `StartedAt` → history reset (old samples don't count
  toward growth).
- **ignore list:** an ignored command never produces a finding.

## Files touched

| File | Change |
|------|--------|
| `internal/detectors/memleak.go` (+ `_test.go`) | New stateful detector |
| `internal/config/config.go` | `MemleakConfig` + `Config.Memleak` field |
| `internal/config/defaults.go` | memleak defaults |
| `internal/config/ignore.go` | `section()` gains `"memleak"` |
| `internal/tui/doctor/update.go` | `[I]` guard allows `"memleak"` |
| `cmd/shrike/doctor.go` | register `NewMemleak()`, `--only`, config map |
| `README.md`, `CHANGELOG.md` | document the detector + one-shot caveat |

## Decisions log (from brainstorm)

1. **Detection model:** hybrid (absolute threshold OR sustained growth), vs. growth-only, vs.
   hog-only. Chosen so the detector is useful on a one-shot scan *and* delivers real leak
   semantics.
2. **Data source:** stateful in-detector RSS sampling across scans, NOT the history file (which
   only stores findings) and NOT an engine/interface change.
3. **Thresholds (defaults):** `rss_threshold` 1 GB, `growth_floor` 250 MB, growth criterion
   20% + 100 MB net over 4 samples.
4. **Growth heuristics are internal constants** (not config) to avoid knob bloat.
5. **No wall-clock** in the detector — reason says "and climbing", keeping tests deterministic.
