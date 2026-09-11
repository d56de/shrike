# GPU Detector Implementation Plan

> Execute inline with test-driven-development and verification-before-completion.

**Goal:** Surface repeated GPU overload and measurable process candidates in doctor and watch.

**Architecture:** Optional contextual detector, bounded ioreg XML collection, per-device state, explicit non-actionable system findings.

**Tech Stack:** Go standard library, existing Cobra/Bubble Tea/TOML stack; no new dependency.

**Spec:** ../specs/2026-09-10-gpu-detector-design.md

## Global constraints

- CGO-free Darwin arm64 and amd64 builds.
- 90% threshold, 30s duration, three observations, maximum 2m gap.
- Raw GPU-time ticks are not percent or wall-clock duration.
- No automatic process actions; system findings never target PID 0.

## Task 1: Collection and detection

- [x] Add XML parser and timestamped detector tests first; run targeted tests and observe missing GPU feature failures.
- [x] Add `core/gpu.go` sample/measurement types and `Finding.System`/`Finding.GPU`.
- [x] Add `sysinfo/gpu.go` bounded collection and `detectors/gpu.go` state machine.
- [x] Add contextual detector dispatch to `core/engine.go`; keep existing detectors compatible.
- [x] Run `go test ./internal/sysinfo ./internal/detectors ./internal/core`.

## Task 2: Integration and safe interaction

- [x] Add regressions for config, engine registration, notification deduplication, JSON/history and system-row action guards.
- [x] Wire `cmd/shrike/doctor.go`, GPU defaults/config/ignores, detector emoji.
- [x] Extend doctor rendering and action guards; use GPU reasons on the secondary row to retain existing row height.
- [x] Serialize GPU metadata and distinguish system notification identities.
- [x] Run targeted tests, including mixed process/system selection.

## Task 3: Verification and documentation

- [x] Document sampling cadence, unsupported telemetry, configuration and `doctor --only gpu`.
- [x] Run `go test -race ./...`, `go vet ./...`, and Darwin arm64/amd64 builds with `CGO_ENABLED=0`.
- [x] Run the built CLI read-only with isolated config/history against live GPU telemetry.
- [x] Review diff and requirements; report measurement limits and invocation.

## Verification results

- `go test -race ./...`: passed.
- `go vet ./...`: passed.
- `golangci-lint run ./...`: passed, zero issues.
- `git diff --check`: passed.
- CGO-free Darwin arm64 and amd64 builds: passed. The sandbox emitted a
  nonfatal module stat-cache permission warning; both commands exited 0.
- Live GPU provider check: read device utilization, 22 client records and two
  advancing counters without root. Real driver layout is an `AppUsage` array;
  client totals preserve uint64 precision and changing entry counts invalidate
  attribution across that interval.
- Built arm64 CLI with isolated temporary config/history: `doctor --only gpu
  --json` exited 0 under current low GPU load. Reading the process list required
  the approved execution outside the sandbox.
- Independent review found no blocking issues. Device focus across refresh and
  retaining a same-named system warning when ignoring a candidate are covered
  by additional regression tests.
- The original nearly-100% incident was not reproduced; sustained overload and
  candidate attribution are verified with deterministic timestamped fixtures.

Run the changed source from the repository:

```sh
go run ./cmd/shrike watch --interval 5s
```

This does not replace the existing installed `shrike` binary or install a
LaunchAgent. Normal watch notifications and history apply while it runs.
