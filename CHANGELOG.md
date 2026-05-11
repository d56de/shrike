# Changelog

All notable changes to `shrike` are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [v0.3.0] — 2026-05-11

### Added

- **Scrollable kill-confirm modal.** Bulk-killing a herd of many processes (e.g. 30 Chrome renderers) no longer overflows the modal frame. `↑` / `↓` / `PgUp` / `PgDn` / `Home` / `End` scroll the target list; `↑ N more above` / `↓ N more below` indicators show how much is off-screen and the footer surfaces `[↑/↓] scroll` when needed.
- **Auto-refresh.** New `[ui] auto_refresh_interval` config setting (e.g. `"5s"`) makes the doctor TUI silently re-run detectors on a cadence. Press `[a]` to toggle at runtime; the status header shows `auto: 5s` / `auto: off`. Rescans skip while a modal is open or an action is running, so the chain never piles up. The cursor follows its PID across rescans instead of jumping to row 0.
- **Richer process info modal.** Pressing `[i]` now lazy-loads extra per-process detail on top of the static fields: working directory, thread count, I/O bytes read/written, full ancestor chain (`launchd(1) → Terminal(456) → zsh(789) → ...`), and (for herds) the helper-children count. The static rows stay visible during the 50–300 ms fetch so the panel never blanks; a spinner sits under the new `─ details ─` divider until data arrives. A stale-PID guard discards late responses if the user navigated to a different cursor while a fetch was in flight.
- **`shrike stats` subcommand.** GitHub-style activity heatmap rendered from `history.jsonl`, defaulting to the last 13 weeks. Colour ramp is quartile-based on non-zero days so the gradient adapts to the user's actual activity scale. Flags: `--weeks 1-104` window length, `--metric {scans,findings,high}` colour driver. Summary line below the grid reports total scans, total findings, active days, longest streak, current streak.

### Changed

- **Initial scan spinner** brands the activity line as `Shrike — scanning processes…` with the `Shrike` wordmark in accent purple, instead of the plain `shrike: scanning processes…` text.

### Fixed

- **`NaN%` CPU values** in the overview, info panel, kill-confirm modal, JSON output, and history file. `gopsutil` returns `NaN` for processes too young to have a CPU-delta sample (and occasionally for zombies); these are now clamped to `0` at ingest in `sysinfo/darwin.go`. Same guard applied to `MemPercent`.
- **List collapses after bulk-kill.** Previously the doctor TUI silently rescanned after every action, causing remaining rows to jump up and `Cursor` / `Offset` / `Selected` to reset — disorienting after killing several herd children at once. Now the action result is recorded in `m.KilledPIDs`, the affected rows render with a `✕` marker + strikethrough, and the list layout stays exactly where it was. Press `R` for an explicit rescan when you're ready for fresh data.
- **Visual gap in CPU bars.** The empty portion of CPU bars used to be drawn with `░` (LIGHT SHADE), whose dotted fill leaves a vertical seam against the adjacent `█` (FULL BLOCK) — especially obvious at low percentages like 1% or 20%. Bars now use a single `█` glyph throughout, foregrounded with the severity colour for the filled portion and a dark grey for the empty portion. The sub-cell partial-block boundary now carries a matching background so it blends seamlessly into the empty cells.

## [v0.2.0] — 2026-05-01

### Added

- **Scrollable findings list.** The doctor TUI now scrolls when the list does not fit in the terminal: arrows auto-track the cursor, plus `PgUp` / `PgDn` for paging and `Home` / `End` to jump to first/last. Indicators show how many findings are above/below the viewport.
- **Footer wrap.** The keyhint footer breaks across multiple lines on narrow terminals so the frame no longer overflows the bottom of the screen.
- **Ignore lists for zombie and herd detectors** (`[zombie] ignore = […]`, `[herd] ignore = […]` in `config.toml`). Defaults now include `AdpSDKUtil`, `AdpFusionMan`, `AdSSO` so Autodesk Fusion's long-lived helpers don't surface as zombies whose parent-kill would terminate the running app.
- **`shrike doctor --threshold N`** — override the runaway CPU threshold (in %) for a single run without editing `config.toml`.

### Changed

- **Kill-confirm modal** now renders zombie parent-redirects un-truncated and adds an explicit warning (`⚠ Parent process will be signalled — may terminate a running GUI app`) so it is always obvious which PID will actually be killed.
- The "no suspicious processes" empty state replaces the `🎉` emoji with a green `✓` glyph for a more terminal-native feel.

### Fixed

- Frame overflow on small terminals when too many findings were detected — previously the bottom of the frame and the keyhints were clipped off-screen.

[v0.3.0]: https://github.com/d56de/shrike/releases/tag/v0.3.0
[v0.2.0]: https://github.com/d56de/shrike/releases/tag/v0.2.0

## [v0.1.0] — 2026-04-21

Initial public release. Pre-alpha — interfaces may still shift before v1.0.

### Added

- `shrike doctor` — interactive Bubble Tea TUI for triaging suspicious processes.
- `shrike doctor --json` — headless JSONL output for scripting and CI. Exit code `0`/`1`/`2` for no findings / findings present / error.
- `shrike doctor --only runaway,zombie,herd` — run a subset of detectors.
- `shrike log [--since 24h] [--pid N]` — read JSONL run history.
- `shrike config` and `shrike config edit` — print or edit effective configuration.
- `shrike version` — print build version.
- **Detectors:**
  - 🔥 **Runaway** — high CPU × long elapsed time, scored as `CPU% × log10(hours+10)`.
  - 🧟 **Zombie** — processes stuck in `Z` or `T` state.
  - 👥 **Herd** — aggregated view of helper-process groups (Chrome renderers, Figma helpers, Claude sessions, …).
- **Actions:**
  - `[i]` info — full process details + open files.
  - `[s]` sample — `sample(1)` for 5s with parsed hottest call stacks.
  - `[k]` kill — SIGTERM, escalating to SIGKILL after 3s.
  - `[K]` kill immediately (SIGKILL, no escalation).
  - `[r]` renice to `+10`.
- TOML config at `$XDG_CONFIG_HOME/shrike/config.toml` (fallback `~/.config/shrike/config.toml`).
- JSONL run history at `$XDG_STATE_HOME/shrike/history.jsonl` (macOS fallback `~/Library/Application Support/shrike/`) with size-based rotation.

### Known limitations

- **Unsigned binary.** First launch from Homebrew will trigger macOS Gatekeeper. Workaround: *System Settings → Privacy & Security → "Allow anyway"*. Code signing + notarisation lands in a future release.
- **First CPU sample is lifetime-average.** `gopsutil.CPUPercent` returns the lifetime average on first call rather than instantaneous load. Re-running `shrike doctor` produces a more accurate picture.
- **App-sandboxed processes** may report incomplete metadata.

[v0.1.0]: https://github.com/d56de/shrike/releases/tag/v0.1.0
