# shrike

> Hunt runaway, zombie, and herd processes on your Mac — in seconds, not scrolling.

![shrike demo](docs/demo.gif)

`shrike` is an opinionated macOS TUI that surfaces **suspicious processes** — runaway CPU hogs, zombies, and helper-process herds (think 11 Chrome renderers or 5 Claude sessions) — and lets you act on them with single keystrokes.

Status: pre-alpha (v0.1 release in progress).

## Install

Once released:

```sh
brew install d56de/tap/shrike
```

From source (any Go 1.26+):

```sh
go install github.com/d56de/shrike/cmd/shrike@latest
```

## Usage

```sh
shrike doctor                # interactive TUI — find, inspect, act
shrike doctor --json         # headless JSON findings for scripting
shrike doctor --only runaway # run a specific detector (runaway | zombie | herd)
shrike log --since 24h       # history of previous runs
shrike config                # print effective config + path
shrike config edit           # open config.toml in $EDITOR
```

Exit code of `shrike doctor --json`: `0` no findings, `1` findings present, `2` error.

## Detectors (v0.1)

- **🔥 Runaway** — high CPU × long elapsed time (`CPU% × log10(hours+10)`)
- **🧟 Zombie** — processes stuck in `Z` or `T` state
- **👥 Herd** — aggregated view of helper-process groups (Chrome renderers, Figma helpers, Claude sessions)

## Actions

- `[i]` info — full process details + open files
- `[s]` sample — runs `sample(1)` for 5s, shows hottest call stacks
- `[k]` kill — SIGTERM, escalating to SIGKILL after 3s
- `[K]` kill immediately (SIGKILL, no escalation)
- `[r]` renice to `+10`

## Why not btop / htop / Activity Monitor?

Existing tools show **raw data**. `shrike` applies opinionated heuristics: it surfaces what's *probably wrong* using the combination of CPU × age × state × group-size that humans actually care about. It groups 11 Chrome helpers into one row. It flags the VM that's been saturating a core for 4 days. It finds zombies without a custom awk pipeline.

## Inspired by [Mole](https://github.com/tw93/Mole)

Same pragmatic vibe (one-shot interactive, checkboxes, single-binary) but for processes instead of files.

## Demo GIF

To regenerate `docs/demo.gif`:

```sh
brew install vhs
vhs docs/demo.tape
```

## License

MIT — see [LICENSE](LICENSE).
