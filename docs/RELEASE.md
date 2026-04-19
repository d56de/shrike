# shrike — Release Fact Sheet

_Last updated: 2026-04-19_

## Current State

- **Repo:** `/Users/christian/code/shrike`
- **Branch:** `main`
- **Commits:** 38
- **Module:** `github.com/d56de/shrike`
- **Go version:** 1.26
- **Planned first release:** `v0.1.0`

### What's implemented (29/29 tasks, Phases 1–5 done, Phase 6 in progress)

- `shrike doctor` — interactive Bubble Tea TUI
- `shrike doctor --json` — headless JSON for scripting / CI (exit 1 on findings)
- `shrike doctor --only runaway,zombie,herd` — select detectors
- `shrike log [--since 24h] [--pid N]` — read JSONL history
- `shrike config [edit]` — print / edit effective config
- `shrike version`
- 3 detectors: Runaway (🔥 CPU×time), Zombie (🧟 state Z/T), Herd (👥 grouped helpers)
- 4 actions: info, sample (macOS `sample(1)` + parser), kill (SIGTERM→SIGKILL), renice +10
- JSONL history at `$XDG_STATE_HOME/shrike/history.jsonl` (macOS fallback `~/Library/Application Support/shrike/`) with size-based rotation
- TOML config at `$XDG_CONFIG_HOME/shrike/config.toml` (fallback `~/.config/shrike/config.toml`)

### What's missing

- **6.3** CHANGELOG.md
- **6.4** Code signing + notarisation (deferred, requires Apple Developer account $99/yr)
- Demo GIF (`docs/demo.gif` — generate locally with `vhs docs/demo.tape`)

## Infrastructure in Place

- `.goreleaser.yml` — darwin amd64+arm64 builds, SHA256 checksums, Homebrew formula for `d56de/homebrew-tap`, filtered GitHub changelog
- `.github/workflows/test.yml` — runs on every push/PR: `go vet`, `go test -race`, golangci-lint
- `.github/workflows/release.yml` — triggered on `v*` tag: tests then `goreleaser release --clean`
- `.golangci.yml` — v2 schema, strict linters enabled (errcheck, govet, staticcheck, unused, revive, gosec, misspell, gofumpt, goimports)

## Pre-Release Checklist (one-time GitHub setup)

Do these before the first `git push`:

- [ ] **Register GitHub user/org** `d56de` (already confirmed 200 OK)
- [ ] **Create repo** `d56de/shrike` (public, empty — do NOT initialise with README/LICENSE, the local repo already has them)
- [ ] **Create repo** `d56de/homebrew-tap` (public, empty)
- [ ] **Generate Fine-grained PAT**:
  - Go to: github.com → Settings → Developer settings → Personal access tokens → Fine-grained
  - Resource owner: `d56de`
  - Repository access: **Only selected** → `d56de/homebrew-tap`
  - Permissions: `Contents: write`
  - Expiration: 1 year (renew via `brew reinstall` won't fail; formula update PRs just won't push)
- [ ] **Add secret** in `d56de/shrike`:
  - Settings → Secrets and variables → Actions → New repository secret
  - Name: `HOMEBREW_TAP_TOKEN`
  - Value: the PAT from previous step

## First Push + Release

```bash
cd ~/code/shrike

# 1. Wire remote
git remote add origin git@github.com:d56de/shrike.git
git push -u origin main

# 2. Tag and push — triggers release workflow
git tag -a v0.1.0 -m "v0.1.0 — MVP release"
git push origin v0.1.0

# 3. Watch CI (optional)
gh run watch --exit-status
```

### What CI will do

1. Check out repo at the tag
2. Run full test suite with `-race`
3. `goreleaser release --clean` builds darwin/amd64 + darwin/arm64, produces `dist/checksums.txt` + archives, opens a PR (or pushes) to `d56de/homebrew-tap` with the updated formula
4. Creates a GitHub Release attached to the tag

## Post-Release Verification

```bash
brew tap d56de/tap
brew install d56de/tap/shrike
shrike version        # should print: shrike 0.1.0
shrike doctor --json  # produce at least one JSON line if the system has findings
```

## Troubleshooting

- **Tag already exists remote:** delete remote tag `git push --delete origin v0.1.0`, fix locally, re-push.
- **Release workflow fails on formula push:** check `HOMEBREW_TAP_TOKEN` has `contents: write` on `d56de/homebrew-tap`.
- **First launch quarantine dialog** (unsigned binary): macOS will refuse to run the binary from Homebrew the first time. User workaround: System Settings → Privacy & Security → "Allow anyway". Root fix comes with Task 6.4 (signing + notarisation — requires Apple Dev account).
- **Stale gopsutil cache causing wrong CPU %:** first call to `gopsutil.CPUPercent` returns lifetime-average, not instant. For v0.2, consider sampling twice with a short delay (~200ms).

## Future Work (v0.2+)

- Phase 6.3: `CHANGELOG.md`
- Phase 6.4: Apple code signing + notarisation
- Memory-drift detector (needs multi-snapshot history query)
- `shrike watch` live dashboard
- `shrike doctor --auto` with policy-based auto-actions
- Raycast integration (`d56de/shrike-raycast`)
- Known-bad-actors curated list
- Orphan detector (PPID=1, etime>1h)
- Lund/permissions-aware sysinfo (currently app-sandboxed procs fail silently)
- Proper `procs_scanned` count plumbed from engine

## Reference Paths

| What | Where |
|---|---|
| Design spec | `docs/superpowers/specs/2026-04-19-shrike-design.md` |
| Implementation plan | `docs/superpowers/plans/2026-04-19-shrike-v0.1-mvp.md` |
| This fact sheet | `docs/RELEASE.md` |
| VHS demo tape | `docs/demo.tape` |
| CI test workflow | `.github/workflows/test.yml` |
| CI release workflow | `.github/workflows/release.yml` |
| Goreleaser config | `.goreleaser.yml` |
| Linter config | `.golangci.yml` |

## Commands Cheat Sheet

```bash
# Build locally
GO111MODULE=on go build -o shrike ./cmd/shrike

# Full test suite
GO111MODULE=on go test -race ./...

# Lint
golangci-lint run ./...

# Run TUI against live system
./shrike doctor

# Run headless for scripting
./shrike doctor --json | jq .

# Relaxed thresholds to see more findings
mkdir -p /tmp/shrike-cfg/shrike
cat > /tmp/shrike-cfg/shrike/config.toml <<'EOF'
[runaway]
cpu_threshold = 30.0
min_age = "30m"
ignore = ["WindowServer", "coreaudiod", "mds", "mdworker"]
[herd]
min_size = 3
total_cpu_threshold = 10.0
EOF
XDG_CONFIG_HOME=/tmp/shrike-cfg ./shrike doctor

# Snapshot goreleaser locally (needs: brew install goreleaser)
goreleaser release --snapshot --clean
```
