# Changelog

All notable changes to `shrike` are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
