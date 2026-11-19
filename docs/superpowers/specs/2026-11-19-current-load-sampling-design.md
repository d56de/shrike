# Current-load sampling (CPU + GPU)

Short design — small change, approved 2026-11-19.

## Intent

shrike missed real resource hogs (workmux dashboards at ~20 % CPU for days, Spline
instances). Root cause: every number it judges by is the wrong kind of average.

- **CPU:** `sysinfo.Provider.Snapshot()` creates fresh gopsutil `Process` objects
  and calls `CPUPercent()` once. Without a prior sample, gopsutil divides total
  CPU time by process lifetime. Old processes look calmer than they are; recent
  spikes vanish.
- **GPU:** the detector gates on ioreg `Device Utilization %`, which reads a
  constant 47–80 % on an idle M1 (see the known artifact: Tiler/Renderer/Device
  identical). Per-client `accumulatedGPUTime` is already parsed but used only
  for attribution, and only across two scans — a one-shot `doctor --only gpu`
  can never name a process.

This change makes shrike see **current** load. Thresholds and new detectors
(sustained moderate load, zombie noise, detached tmux, duplicate apps) are
follow-ups, not part of this.

## Goals

- `ProcessInfo.CPUPercent` = CPU used during a ~1 s window inside `Snapshot`.
- New `ProcessInfo.GPUPercent` = GPU time during the same window / window length.
- GPU detector decides on the **sum of per-process GPU percent**, not on
  `Device Utilization %`.
- Works for a one-shot `doctor` run; `watch` and TUI auto-refresh get it for free.

## Non-goals

- Changing thresholds, scores or detector set.
- Persisting samples between scans.

## Anti-goals

- A finding driven by the ioreg utilization artifact.
- Mis-attributing a recycled PID's counters to a new process.

## Approaches

1. **Two samples inside `Snapshot`, ~1 s apart** — *picked.* Self-contained,
   correct for one-shot runs, one place to change; cost is +1 s per scan.
2. Keep counters between scans (stateful provider) — gives 60 s averages under
   `watch`, but needs state and still needs approach 1 for the first scan.
   Sustained load is better handled later via history.
3. Parse `top -l 2` — external output format, no GPU, rejected.

## Design

- `Snapshot`: list processes, read CPU times + GPU client ticks, wait the window,
  read again; percent = delta / window. Match samples by **PID + start time**;
  processes absent from the first read get 0.
- GPU window shares the CPU window (one ioreg read per sample point).
- `Device Utilization %` stays only as a displayed raw value, never in a decision.
- The GPU detector keeps its repeated-observation confirmation, fed by the new
  total; it names the top processes directly from `GPUPercent`.
- The window is injectable (clock/sleeper) so tests need no real waiting.

## Tests

- Old process with high current load reports the current value (CPU and GPU),
  not the lifetime mean.
- Idle GPU with `Device Utilization %` = 80 produces no GPU finding.
- PID reused between the two reads → 0, not a bogus delta.

## Open assumption

`accumulatedGPUTime` is assumed to count **nanoseconds** (a manual measurement
gave plausible 9 % / 5 %). Plan includes a check: under real GPU load the summed
GPU percent must stay ≤ 100 % of the window.

## Left to the builder

Exact window length (≈1 s), where the GPU sample plumbing lives (`sysinfo`
vs. engine), and how the TUI labels the raw utilization value.
