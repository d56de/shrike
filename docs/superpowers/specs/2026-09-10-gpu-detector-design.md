# GPU overload detection

Approved direction: detect sustained GPU overload, identify active processes when
possible, and retain a system warning when attribution is unavailable. The user
observed nearly 100% GPU in btop and a hot Mac, without a known process name.

## Measurement and detection

- Read `/usr/sbin/ioreg -a -l -r -c IOAccelerator` once per scan, with a two-second
  context timeout. Parse XML with the standard library; retain CGO-free builds.
- Track each GPU separately by registry ID. Read `Device Utilization %` and each
  client's `AppUsage.accumulatedGPUTime`, registry ID and creator PID.
- Default threshold: 90%; minimum duration: 30 seconds; at least three high
  observations. A low/invalid/missing observation or gap over two minutes resets
  the duration. This establishes repeated observations, not continuous coverage
  between scans. Watch's 60-second default warns on the third scan (about 120s);
  `watch --interval 5s` provides faster detection. No background sampling service.
- A single high observation yields a medium system finding, explicitly awaiting
  confirmation. Confirmed overload yields a high system finding and up to three
  active process candidates, ranked by GPU counter delta. Candidates require the
  same client registry ID, PID and process start time across consecutive scans.
- Counter deltas are reported as raw GPU-time ticks, never GPU utilization
  percentages or fabricated wall-clock units. Counter resets, new clients, PID
  reuse and exited processes cannot become candidates. High activity is evidence
  of work, not proof of a hung process.
- Process ignore lists suppress candidate rows, never the system overload warning.
  Failed/unsupported telemetry produces a low system diagnostic so an unavailable
  sensor is distinguishable from a clean result. Disabling `[gpu] enabled` removes
  collection and findings entirely.

## Integration

Keep the process-only detector API, adding an optional contextual detector method
for bounded GPU I/O. GPU findings carry structured device and measurement data,
and an explicit system scope for findings with no actionable process. Existing
process findings retain their current wire format; GPU metadata is additive.

Doctor renders GPU rows with GPU values and their reason instead of CPU/RSS/age.
System rows cannot be selected for kill, renice, pause, sample, info or ignore;
mixed selections act only on real processes. GPU candidates use existing explicit
process actions and the `[gpu] ignore` section. Watch notifications deduplicate
system warnings by GPU identity, and history/JSON preserve GPU measurements.

## Validation

Use synthetic XML and timestamped samples to verify unsupported data, invalid
numbers, multiple devices, nested clients, transient spikes, sustained overload,
recovery, gaps, counter reset/churn, PID reuse and ignores. Test system-action
guards, notification deduplication and JSON/history serialization. Run race tests,
vet, CGO-free builds for both Darwin architectures, and a read-only live smoke
scan. A naturally occurring hung GPU process is not required for deterministic
tests; reproducing the user's exact incident remains unverified.
