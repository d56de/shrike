# Current-load Sampling (CPU + GPU) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** shrike judges processes by CPU and GPU load measured over a ~1 s window inside each scan, instead of lifetime averages (CPU) and the ioreg utilization artifact (GPU).

**Architecture:** A pure function in `internal/sysinfo/load.go` turns two counter readings into per-process CPU % and GPU %. `Provider.Snapshot` takes both readings around one window and fills `ProcessInfo.CPUPercent` and the new `ProcessInfo.GPUPercent`. The GPU detector then decides on the sum of `GPUPercent`; `Device Utilization %` becomes a display-only raw value.

**Tech Stack:** Go 1.26+, gopsutil v4 (`process.TimesWithContext`, `CreateTimeWithContext`), ioreg XML via the existing `GPUProvider`.

**Spec:** `docs/superpowers/specs/2026-11-19-current-load-sampling-design.md`

## Global Constraints

- Sampling window: **1 s** (`DefaultLoadWindow = time.Second`), injectable for tests.
- Match readings by **PID + process start time**; GPU clients additionally by client registry ID and `Channels`. Anything unmatched → 0 %, never a guessed value.
- `Device Utilization %` must not appear in any decision (threshold, severity, score).
- `accumulatedGPUTime` treated as **nanoseconds**. This reverses the 2026-09-10 GPU design ("raw ticks, never percent"); Task 4 is the check that justifies it.
- CPU % range stays `0 – 100 × cores` (existing `ProcessInfo` contract).
- No CGO; no new dependencies.

## Review Focus

1. **Ctrl-C / timeout during the window** — `doctor` must abort promptly, not after the sleep. → Task 2 test `TestSnapshot_CancelDuringWindow`.
2. **Process exits between the two readings** — dropped or 0 %, never a crash or negative value. → Task 1 test `TestComputeLoad_ExitedAndNewProcesses`.
3. **GPU read fails on one of the two points** — CPU values still delivered, all `GPUPercent` = 0, no error for the whole snapshot. → Task 2 test `TestSnapshot_GPUFailureKeepsCPU`.
4. **Counter goes backwards** (driver reset, channel churn) — 0 %, not a wrapped huge delta. → Task 1 test `TestComputeLoad_GPUCounterResetAndChannelChange`.
5. **Scan cost grows with process count** — exactly one window per scan, not one per process. → Task 2 test `TestSnapshot_SleepsOnce`.

---

### Task 1: Pure load computation

**Files:**
- Create: `internal/sysinfo/load.go`
- Test: `internal/sysinfo/load_test.go`

**Interfaces:**
- Produces:
  ```go
  type procReading struct { Started int64; CPUSeconds float64 } // Started = gopsutil CreateTime (ms)
  type loadReading struct { Procs map[int]procReading; GPU []core.GPUClient }
  type procLoad struct { CPUPercent, GPUPercent float64 }
  func computeLoad(a, b loadReading, window time.Duration) map[int]procLoad
  ```
  Only PIDs present in both readings with equal `Started` appear in the result. GPU % per PID = Σ over clients matched in `a` and `b` (same ID, PID, Channels, `b.Ticks >= a.Ticks`) of `(b.Ticks − a.Ticks) / window.Nanoseconds() × 100`, added only if the PID's process matched.

- [ ] **Step 1: Write the failing tests** in `load_test.go` (package `sysinfo`), `window := time.Second`:
  - `TestComputeLoad_CurrentNotLifetime`: pid 1, Started 1000, CPUSeconds 5000 → 5000.25 ⇒ `CPUPercent` == 25 (±0.001).
  - `TestComputeLoad_ExitedAndNewProcesses`: pid 2 only in `a`, pid 3 only in `b` ⇒ neither key in result.
  - `TestComputeLoad_PIDReuse`: pid 4 Started 1000 in `a`, Started 2000 in `b` ⇒ key absent.
  - `TestComputeLoad_GPUPercent`: pid 1 matched, client {ID 9, PID 1, Channels 2} Ticks 1_000_000_000 → 1_090_000_000 ⇒ `GPUPercent` == 9; second client of pid 1 {ID 10} +50_000_000 ⇒ total 14.
  - `TestComputeLoad_GPUCounterResetAndChannelChange`: Ticks decrease ⇒ 0; Channels 2 → 3 ⇒ 0; client PID unmatched process ⇒ no key.
  - `TestComputeLoad_NegativeCPUDelta`: CPUSeconds decreases ⇒ `CPUPercent` == 0.

- [ ] **Step 2: Run** `go test ./internal/sysinfo -run TestComputeLoad -v` — Expected: FAIL, `undefined: computeLoad`.

- [ ] **Step 3: Implement** the types and `computeLoad` in `load.go`. Clamp negative deltas to 0; skip non-positive windows (return empty map).

- [ ] **Step 4: Run** the same command — Expected: PASS.

- [ ] **Step 5: Commit** `feat: compute per-process CPU/GPU load from two readings`

---

### Task 2: Two-reading Snapshot

**Files:**
- Modify: `internal/sysinfo/darwin.go` (Provider, `Snapshot`, `convert`)
- Modify: `internal/core/process.go` — add `GPUPercent float64 // share of one GPU over the sampling window, 0–100 per device`
- Modify: `cmd/shrike/doctor.go:215` — `sysinfo.New()` keeps working (defaults below)
- Test: `internal/sysinfo/darwin_test.go`

**Interfaces:**
- Consumes: `computeLoad`, `loadReading`, `procReading` (Task 1); `core.GPUSnapshotter`.
- Produces:
  ```go
  const DefaultLoadWindow = time.Second
  type Provider struct {
      GPU    core.GPUSnapshotter                        // nil = no GPU load
      Window time.Duration                              // 0 = DefaultLoadWindow
      Sleep  func(context.Context, time.Duration) error // nil = ctx-aware timer
  }
  func New() Provider // GPU: GPUProvider{}, Window: DefaultLoadWindow
  ```
  `ProcessInfo.CPUPercent` and `.GPUPercent` come from `computeLoad`; processes without a match get 0. `convert` no longer calls `CPUPercentWithContext`.

- [ ] **Step 1: Write the failing tests** in `darwin_test.go`:
  - `TestSnapshot_SleepsOnce`: Provider with `Sleep` counting calls and returning nil ⇒ exactly 1 call, argument == `Window`.
  - `TestSnapshot_CancelDuringWindow`: `Sleep` returns `context.Canceled` ⇒ `Snapshot` returns an error wrapping `context.Canceled`.
  - `TestSnapshot_GPUFailureKeepsCPU`: `GPU` fake returning an error ⇒ no error, every `GPUPercent` == 0, own PID present.
  - `TestSnapshot_MeasuresWindow`: real `Sleep`, `Window` 500 ms, a goroutine busy-spins for the whole call ⇒ own PID `CPUPercent` ≥ 50.
  - Keep `TestSnapshot_NoNaNPercentages` and `TestSnapshot_ReturnsCurrentProcess` green (extend NaN check to `GPUPercent`).

- [ ] **Step 2: Run** `go test ./internal/sysinfo -run TestSnapshot -v` — Expected: FAIL (unknown fields `GPU`, `Window`, `Sleep`).

- [ ] **Step 3: Implement.** Order in `Snapshot`: list processes → reading A (CPU times via `TimesWithContext` → User+System, `CreateTimeWithContext`; GPU via `p.GPU.Snapshot`, flatten all devices' clients) → `Sleep(ctx, window)` → reading B over the **same** process list → `computeLoad` → `convert` each process and set both percents. A GPU error at either point ⇒ empty GPU slices for both readings.

- [ ] **Step 4: Run** `go test ./internal/sysinfo -v` and `go build ./...` — Expected: PASS, build OK.

- [ ] **Step 5: Commit** `feat: measure CPU and GPU load over a 1s window per scan`

---

### Task 3: GPU detector decides on process GPU load

**Files:**
- Modify: `internal/detectors/gpu.go`
- Modify: `internal/core/gpu.go` — `GPUFinding`: add `LoadPercent float64 \`json:"load_percent"\``; drop `DeltaTicks`/`IntervalSeconds`; fix the `GPUClient.Ticks` comment (nanoseconds, see Task 4)
- Modify: `internal/tui/doctor/gpu.go:14-22` — system row shows `LoadPercent` (raw `Utilization` only as `(ioreg n%)` suffix when present); candidate row shows `PID %d · %.1f%% GPU`
- Test: `internal/detectors/gpu_test.go`, `internal/detectors/gpu_simulation_test.go` (rewrite to the new inputs)

**Interfaces:**
- Consumes: `ProcessInfo.GPUPercent` (Task 2).
- Produces: one system finding per scan when total ≥ threshold, `GPU.LoadPercent` = Σ `GPUPercent` of live, non-zombie processes; candidates = top 3 by `GPUPercent` (ignore list applied, ties by PID), severity high only when confirmed. The source read stays only for availability, device name/ID (first device) and the raw `Utilization`.

- [ ] **Step 1: Rewrite the tests**:
  - `TestGPUIgnoresUtilizationArtifact`: device `Utilization` 80, all `GPUPercent` 0, threshold 50 ⇒ no finding with `Severity` ≥ medium.
  - `TestGPUSustainedLoadAndCandidate`: three scans 15 s apart, procs {pid 1: 70 %, pid 2: 25 %}, threshold 90 ⇒ scan 3 has a high system finding with `LoadPercent` 95 and candidates in order pid 1, pid 2.
  - `TestGPUResetsOnLowMissingErrorAndLongGap`: keep the existing cases, driven by `GPUPercent` instead of `Utilization`.
  - `TestGPUIgnoredAndZombieNotCandidates`: ignored command and zombie still count toward the total, never listed as candidates.
  - `TestGPUUnsupportedIsVisible`: unchanged.
  - `gpu_simulation_test.go`: same scenario shape, inputs via `GPUPercent`.

- [ ] **Step 2: Run** `go test ./internal/detectors -run TestGPU -v` — Expected: FAIL.

- [ ] **Step 3: Implement.** Remove per-client tick history (`gpuHistory.clients`, `gpuCounter`); keep `highSince`/`samples`/gap reset logic keyed by the first device's ID. Reason strings: `"GPU %.1f%% (sum of processes); …"` and `"GPU-active candidate: %.1f%% GPU over the last scan window; may be legitimate work"`.

- [ ] **Step 4: Run** `go test ./... ` — Expected: PASS (incl. TUI tests).

- [ ] **Step 5: Commit** `feat: base GPU overload on per-process load, not ioreg utilization`

---

### Task 4: Unit check against Activity Monitor + docs

**Files:**
- Modify: `README.md` (GPU overload section), `CHANGELOG.md` (Unreleased)

- [ ] **Step 1: Manual check (human):** run a GPU-heavy app (e.g. a Spline scene rotating, or a WebGL demo), then `go run ./cmd/shrike doctor --json --only gpu` and compare the top candidate's percent with Activity Monitor's **% GPU** column. Pass: within ±5 points, and the system total ≤ 100. If off by a constant factor, fix the unit in `computeLoad` and its tests before continuing.

- [ ] **Step 2: Docs.** README: GPU load = sum of per-process GPU time over a 1 s window; ioreg `Device Utilization %` is shown raw and ignored for decisions (reason: constant 47–80 % when idle on M1). CHANGELOG: CPU now current-window, not lifetime; GPU decision basis changed; JSON `delta_ticks`/`interval_seconds` replaced by `load_percent`; each scan takes ~1 s longer.

- [ ] **Step 3: Verify** `go vet ./... && go test ./...` — Expected: no output from vet, all PASS.

- [ ] **Step 4: Commit** `docs: describe current-load sampling and GPU basis change`
