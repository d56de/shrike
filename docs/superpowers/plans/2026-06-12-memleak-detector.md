# memleak Detector Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a fourth detector, `memleak` (🧠), to `shrike doctor` that flags memory hogs (RSS over a threshold) and leaks (RSS growing steadily across scans).

**Architecture:** A stateful `*Memleak` detector implementing `core.Detector`. It keeps a per-PID RSS ring buffer in memory and accumulates one sample per scan; the engine constructs detectors once and reuses them across rescans, so growth detection works without any engine/interface or history-file change. A process is flagged if `RSS ≥ rss_threshold` (hog, fires one-shot) OR it shows sustained growth above a floor (leak). Plus config, ignore-from-TUI synergy, and engine wiring.

**Tech Stack:** Go 1.26, standard `go test`, BurntSushi/toml.

**Spec:** `docs/superpowers/specs/2026-06-12-memleak-detector-design.md`

**Conventions for every commit:** run `gofmt`/`goimports`, then `go test ./...`. Commit messages use `feat:` / `test:` / `docs:`. No `Co-Authored-By` trailer (attribution disabled globally for this repo).

---

## Task 1: The memleak detector

**Files:**
- Create: `internal/detectors/memleak.go`
- Test: `internal/detectors/memleak_test.go`

The detector reads its config from the `core.DetectorConfig` map (keys `rss_threshold` as `uint64` bytes, `min_age` as `time.Duration`, `ignore` as `[]string`), exactly like `runaway.go`. Tests pass that map directly, so this task is independent of the config-struct task.

- [ ] **Step 1: Write the failing tests**

Create `internal/detectors/memleak_test.go`:

```go
package detectors

import (
	"strings"
	"testing"
	"time"

	"github.com/d56de/shrike/internal/core"
)

func mb(n uint64) uint64 { return n * 1024 * 1024 }

func memCfg() core.DetectorConfig {
	return core.DetectorConfig{
		"rss_threshold": uint64(1024) * 1024 * 1024, // 1 GiB
		"min_age":       5 * time.Minute,
		"ignore":        []string(nil),
	}
}

func TestMemleak_HogFiresOnSingleScan(t *testing.T) {
	m := NewMemleak()
	procs := []core.ProcessInfo{
		{PID: 1, Command: "bloaty", RSS: mb(1500), ElapsedTime: time.Hour, StartedAt: time.Unix(1000, 0)},
	}
	got := m.Detect(procs, memCfg())
	if len(got) != 1 {
		t.Fatalf("expected 1 hog finding, got %d", len(got))
	}
	if got[0].Detector != "memleak" {
		t.Errorf("detector = %q, want memleak", got[0].Detector)
	}
}

func TestMemleak_BelowThresholdNoHog(t *testing.T) {
	m := NewMemleak()
	procs := []core.ProcessInfo{{PID: 1, Command: "x", RSS: mb(300), ElapsedTime: time.Hour, StartedAt: time.Unix(1000, 0)}}
	if got := m.Detect(procs, memCfg()); len(got) != 0 {
		t.Fatalf("expected no finding for a 300MB single sample, got %+v", got)
	}
}

func TestMemleak_YoungHogGatedByMinAge(t *testing.T) {
	m := NewMemleak()
	procs := []core.ProcessInfo{{PID: 1, Command: "x", RSS: mb(2000), ElapsedTime: time.Minute, StartedAt: time.Unix(1000, 0)}}
	if got := m.Detect(procs, memCfg()); len(got) != 0 {
		t.Fatalf("expected a young huge proc to be gated by min_age, got %+v", got)
	}
}

func TestMemleak_LeakFiresAfterSustainedGrowth(t *testing.T) {
	m := NewMemleak()
	st := time.Unix(1000, 0)
	seq := []uint64{mb(400), mb(500), mb(600), mb(700)} // below the 1GB hog threshold
	for i, r := range seq {
		got := m.Detect([]core.ProcessInfo{
			{PID: 1, Command: "leaky", RSS: r, ElapsedTime: time.Hour, StartedAt: st},
		}, memCfg())
		if i < 3 && len(got) != 0 {
			t.Fatalf("sample %d: expected no finding yet, got %+v", i, got)
		}
		if i == 3 {
			if len(got) != 1 {
				t.Fatalf("sample 3: expected a leak finding, got %d", len(got))
			}
			if got[0].Severity < core.SeverityMedium {
				t.Errorf("expected >= medium severity, got %v", got[0].Severity)
			}
			if !strings.Contains(got[0].Reason, "climbing") {
				t.Errorf("expected 'climbing' in reason, got %q", got[0].Reason)
			}
		}
	}
}

func TestMemleak_GCJitterNoLeak(t *testing.T) {
	m := NewMemleak()
	st := time.Unix(1000, 0)
	seq := []uint64{mb(400), mb(450), mb(380), mb(420), mb(400), mb(410)} // oscillates, no net growth
	for _, r := range seq {
		got := m.Detect([]core.ProcessInfo{
			{PID: 1, Command: "gc", RSS: r, ElapsedTime: time.Hour, StartedAt: st},
		}, memCfg())
		if len(got) != 0 {
			t.Fatalf("expected no leak for GC jitter, got %+v", got)
		}
	}
}

func TestMemleak_PIDReuseResetsHistory(t *testing.T) {
	m := NewMemleak()
	old := time.Unix(1000, 0)
	for _, r := range []uint64{mb(400), mb(500), mb(600)} {
		m.Detect([]core.ProcessInfo{{PID: 1, Command: "a", RSS: r, ElapsedTime: time.Hour, StartedAt: old}}, memCfg())
	}
	// PID 1 now owned by a different process (new StartedAt).
	got := m.Detect([]core.ProcessInfo{
		{PID: 1, Command: "b", RSS: mb(700), ElapsedTime: time.Hour, StartedAt: time.Unix(2000, 0)},
	}, memCfg())
	if len(got) != 0 {
		t.Fatalf("expected reset history (no growth) after PID reuse, got %+v", got)
	}
}

func TestMemleak_EvictionResetsHistory(t *testing.T) {
	m := NewMemleak()
	st := time.Unix(1000, 0)
	for _, r := range []uint64{mb(400), mb(500), mb(600)} {
		m.Detect([]core.ProcessInfo{{PID: 1, Command: "a", RSS: r, ElapsedTime: time.Hour, StartedAt: st}}, memCfg())
	}
	m.Detect(nil, memCfg()) // process gone this scan → evicted
	got := m.Detect([]core.ProcessInfo{
		{PID: 1, Command: "a", RSS: mb(700), ElapsedTime: time.Hour, StartedAt: st},
	}, memCfg())
	if len(got) != 0 {
		t.Fatalf("expected eviction to reset history (no leak on reappearance), got %+v", got)
	}
}

func TestMemleak_IgnoreList(t *testing.T) {
	m := NewMemleak()
	cfg := memCfg()
	cfg["ignore"] = []string{"bloaty"}
	procs := []core.ProcessInfo{{PID: 1, Command: "bloaty", RSS: mb(2000), ElapsedTime: time.Hour, StartedAt: time.Unix(1000, 0)}}
	if got := m.Detect(procs, cfg); len(got) != 0 {
		t.Fatalf("expected ignored command to produce no finding, got %+v", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/detectors/ -run TestMemleak -v`
Expected: FAIL — `undefined: NewMemleak`.

- [ ] **Step 3: Write the implementation**

Create `internal/detectors/memleak.go`:

```go
package detectors

import (
	"fmt"
	"slices"
	"time"

	"github.com/d56de/shrike/internal/core"
)

const (
	memleakMaxSamples    = 10
	memleakMinSamples    = 4
	memleakGrowthMB      = 100
	memleakGrowthPct     = 0.20
	memleakGrowthFloorMB = 250
	mib                  = 1024 * 1024
)

// Memleak flags processes that use a lot of memory (hog) or whose RSS grows
// steadily across scans (leak). It is STATEFUL: it accumulates a per-PID RSS
// sample history across Detect calls so growth can be observed over time.
//
// Not safe for concurrent Detect calls on the same instance. The engine calls
// each detector's Detect once per Run (one goroutine) and Runs are sequential,
// so this is safe in practice.
type Memleak struct {
	hist map[int]*pidHistory
}

type pidHistory struct {
	startedAt time.Time // from ProcessInfo.StartedAt — detects PID reuse
	rss       []uint64  // ring buffer, newest last, capped at memleakMaxSamples
}

// NewMemleak returns a ready-to-use stateful Memleak detector.
func NewMemleak() *Memleak { return &Memleak{hist: map[int]*pidHistory{}} }

// Name implements core.Detector.
func (*Memleak) Name() string { return "memleak" }

// Emoji implements core.Detector.
func (*Memleak) Emoji() string { return "🧠" }

// Detect samples each process's RSS and emits findings for hogs and leaks.
func (m *Memleak) Detect(procs []core.ProcessInfo, cfg core.DetectorConfig) []core.Finding {
	threshold, _ := cfg["rss_threshold"].(uint64)
	if threshold == 0 {
		threshold = 1024 * mib
	}
	minAge, _ := cfg["min_age"].(time.Duration)
	if minAge == 0 {
		minAge = 5 * time.Minute
	}
	ignore, _ := cfg["ignore"].([]string)

	growthFloor := uint64(memleakGrowthFloorMB) * mib
	growthBytes := uint64(memleakGrowthMB) * mib

	// Evict tracked PIDs that are no longer present (process exited).
	live := make(map[int]bool, len(procs))
	for _, p := range procs {
		live[p.PID] = true
	}
	for pid := range m.hist {
		if !live[pid] {
			delete(m.hist, pid)
		}
	}

	var out []core.Finding
	for _, p := range procs {
		if slices.Contains(ignore, p.Command) {
			delete(m.hist, p.PID) // stop tracking ignored processes
			continue
		}

		h := m.hist[p.PID]
		switch {
		case h == nil:
			if p.RSS < growthFloor {
				continue // below floor and untracked → not interesting yet
			}
			h = &pidHistory{startedAt: p.StartedAt}
			m.hist[p.PID] = h
		case !h.startedAt.Equal(p.StartedAt):
			// PID reuse: a different process now owns this PID.
			h.startedAt = p.StartedAt
			h.rss = h.rss[:0]
		}
		h.rss = append(h.rss, p.RSS)
		if len(h.rss) > memleakMaxSamples {
			h.rss = h.rss[len(h.rss)-memleakMaxSamples:]
		}

		oldest := h.rss[0]
		newest := h.rss[len(h.rss)-1]
		growing := len(h.rss) >= memleakMinSamples &&
			newest >= oldest+growthBytes &&
			float64(newest) >= float64(oldest)*(1+memleakGrowthPct)

		hog := p.RSS >= threshold && p.ElapsedTime >= minAge
		leak := growing && p.RSS >= growthFloor
		if !hog && !leak {
			continue
		}

		score := float64(p.RSS) / (1 << 30) * 50
		var growth uint64
		if growing {
			growth = newest - oldest
			score += 50 + float64(growth)/(1<<30)*150
		}

		out = append(out, core.Finding{
			Process:  p,
			Detector: "memleak",
			Severity: memleakSeverity(score),
			Score:    score,
			Reason:   memleakReason(p.RSS, growing, growth),
		})
	}
	return out
}

func memleakSeverity(score float64) core.Severity {
	switch {
	case score >= 250:
		return core.SeverityCritical
	case score >= 120:
		return core.SeverityHigh
	case score >= 50:
		return core.SeverityMedium
	default:
		return core.SeverityLow
	}
}

func memleakReason(rss uint64, growing bool, growth uint64) string {
	if growing {
		return fmt.Sprintf("%s RSS, +%d MB and climbing", formatBytes(rss), growth/mib)
	}
	return fmt.Sprintf("%s RSS", formatBytes(rss))
}

// formatBytes renders bytes as "2.4 GB" at/above 1 GiB, otherwise "512 MB".
func formatBytes(b uint64) string {
	const gib = 1024 * mib
	if b >= gib {
		return fmt.Sprintf("%.1f GB", float64(b)/float64(gib))
	}
	return fmt.Sprintf("%d MB", b/mib)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/detectors/ -run TestMemleak -v`
Expected: PASS (7 tests).

- [ ] **Step 5: Run the whole detectors suite + vet for regressions**

Run: `go test ./internal/detectors/ -race -count=1 && go vet ./internal/detectors/`
Expected: PASS, no vet output. (Confirms `formatBytes`/`mib` don't collide with existing detector helpers.)

- [ ] **Step 6: Commit**

```bash
gofmt -w internal/detectors/memleak.go internal/detectors/memleak_test.go
git add internal/detectors/memleak.go internal/detectors/memleak_test.go
git commit -m "feat(detectors): add stateful memleak detector (hog + growth)"
```

---

## Task 2: Config — `MemleakConfig` + defaults

**Files:**
- Modify: `internal/config/config.go` (Config struct + new type)
- Modify: `internal/config/defaults.go`
- Test: `internal/config/config_test.go` (add one test)

- [ ] **Step 1: Write the failing test**

Append to `internal/config/config_test.go`:

```go
func TestDefaultConfig_HasMemleakDefaults(t *testing.T) {
	c := DefaultConfig()
	if c.Memleak.RSSThresholdMB != 1024 {
		t.Errorf("expected RSSThresholdMB=1024, got %d", c.Memleak.RSSThresholdMB)
	}
	if time.Duration(c.Memleak.MinAge) != 5*time.Minute {
		t.Errorf("expected MinAge=5m, got %v", time.Duration(c.Memleak.MinAge))
	}
}

func TestLoad_OverridesMemleakThreshold(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	dir := filepath.Join(tmp, "shrike")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "[memleak]\nrss_threshold_mb = 2048\n"
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Memleak.RSSThresholdMB != 2048 {
		t.Errorf("expected 2048, got %d", cfg.Memleak.RSSThresholdMB)
	}
	// Unspecified field keeps its default.
	if time.Duration(cfg.Memleak.MinAge) != 5*time.Minute {
		t.Errorf("expected default MinAge=5m, got %v", time.Duration(cfg.Memleak.MinAge))
	}
}
```

(The test file already imports `os`, `path/filepath`, `testing`, `time`.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/config/ -run 'Memleak' -v`
Expected: FAIL — `c.Memleak` undefined.

- [ ] **Step 3: Add `MemleakConfig` and the `Config.Memleak` field in `internal/config/config.go`**

In the `Config` struct, the current fields are:

```go
	Herd    HerdConfig    `toml:"herd"`
	History HistoryConfig `toml:"history"`
```

Replace with (insert `Memleak` between Herd and History):

```go
	Herd    HerdConfig    `toml:"herd"`
	Memleak MemleakConfig `toml:"memleak"`
	History HistoryConfig `toml:"history"`
```

Then add the new type immediately after the `HerdConfig` type definition:

```go
// MemleakConfig configures the memleak detector.
type MemleakConfig struct {
	RSSThresholdMB int      `toml:"rss_threshold_mb"`
	MinAge         Duration `toml:"min_age"`
	Ignore         []string `toml:"ignore"`
}
```

- [ ] **Step 4: Add the defaults in `internal/config/defaults.go`**

The current default block has:

```go
		Herd: HerdConfig{
			MinSize:           5,
			TotalCPUThreshold: 30.0,
			KnownBadActors:    []string{},
			Ignore:            []string{},
		},
		History: HistoryConfig{
```

Insert the `Memleak` default between Herd and History:

```go
		Herd: HerdConfig{
			MinSize:           5,
			TotalCPUThreshold: 30.0,
			KnownBadActors:    []string{},
			Ignore:            []string{},
		},
		Memleak: MemleakConfig{
			RSSThresholdMB: 1024,
			MinAge:         Duration(5 * time.Minute),
			Ignore:         []string{},
		},
		History: HistoryConfig{
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/config/ -v`
Expected: PASS (the two new tests plus all existing config tests).

- [ ] **Step 6: Commit**

```bash
gofmt -w internal/config/config.go internal/config/defaults.go internal/config/config_test.go
git add internal/config/config.go internal/config/defaults.go internal/config/config_test.go
git commit -m "feat(config): add memleak detector config + defaults"
```

---

## Task 3: ignore-from-TUI synergy (`[I]` works on memleak findings)

**Files:**
- Modify: `internal/config/ignore.go`
- Modify: `internal/tui/doctor/update.go` (the `[I]` guard)
- Test: `internal/config/ignore_test.go`, `internal/tui/doctor/pause_ignore_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/config/ignore_test.go`:

```go
func TestAppendIgnoreAt_MemleakSection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ignore.toml")
	if err := AppendIgnoreAt(path, "memleak", "Electron"); err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	if err := mergeIgnoresAt(path, &cfg); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range cfg.Memleak.Ignore {
		if c == "Electron" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'Electron' merged into memleak ignore, got %v", cfg.Memleak.Ignore)
	}
}
```

Append to `internal/tui/doctor/pause_ignore_test.go`:

```go
func TestIgnore_AllowsMemleakFinding(t *testing.T) {
	m := Model{
		Findings:   []core.Finding{{Detector: "memleak", Process: core.ProcessInfo{PID: 9, Command: "Electron"}}},
		Selected:   map[int]bool{},
		Paused:     map[int]core.ProcessInfo{},
		KilledPIDs: map[int]bool{},
		IgnorePath: filepath.Join(t.TempDir(), "ignore.toml"),
	}
	mm, _ := m.Update(keyRunes('I'))
	m = mm.(Model)
	if m.Mode != ModeConfirmIgnore || m.IgnorePending == nil {
		t.Fatalf("expected [I] to open ignore confirm for a memleak finding, got mode=%v", m.Mode)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/config/ -run TestAppendIgnoreAt_MemleakSection ./internal/tui/doctor/ -run TestIgnore_AllowsMemleak -v`
Expected: FAIL — config: `AppendIgnoreAt(... "memleak" ...)` returns "unknown detector"; doctor: `[I]` is refused (memleak not in the allowed set).

- [ ] **Step 3: Add the memleak section to `internal/config/ignore.go`**

The `ignoreFileData` struct is:

```go
type ignoreFileData struct {
	Runaway sectionIgnore `toml:"runaway"`
	Zombie  sectionIgnore `toml:"zombie"`
	Herd    sectionIgnore `toml:"herd"`
}
```

Replace with (add the Memleak field):

```go
type ignoreFileData struct {
	Runaway sectionIgnore `toml:"runaway"`
	Zombie  sectionIgnore `toml:"zombie"`
	Herd    sectionIgnore `toml:"herd"`
	Memleak sectionIgnore `toml:"memleak"`
}
```

Then in the `section` method, the current switch is:

```go
	switch detector {
	case "runaway":
		return &d.Runaway.Ignore
	case "zombie":
		return &d.Zombie.Ignore
	case "herd":
		return &d.Herd.Ignore
	default:
		return nil
	}
```

Add a `memleak` case before `default`:

```go
	case "herd":
		return &d.Herd.Ignore
	case "memleak":
		return &d.Memleak.Ignore
	default:
		return nil
```

Also update `mergeIgnoresAt` so memleak ignores merge into the config. The current body is:

```go
	cfg.Runaway.Ignore = mergeDedup(cfg.Runaway.Ignore, d.Runaway.Ignore)
	cfg.Zombie.Ignore = mergeDedup(cfg.Zombie.Ignore, d.Zombie.Ignore)
	cfg.Herd.Ignore = mergeDedup(cfg.Herd.Ignore, d.Herd.Ignore)
	return nil
```

Add a memleak line:

```go
	cfg.Runaway.Ignore = mergeDedup(cfg.Runaway.Ignore, d.Runaway.Ignore)
	cfg.Zombie.Ignore = mergeDedup(cfg.Zombie.Ignore, d.Zombie.Ignore)
	cfg.Herd.Ignore = mergeDedup(cfg.Herd.Ignore, d.Herd.Ignore)
	cfg.Memleak.Ignore = mergeDedup(cfg.Memleak.Ignore, d.Memleak.Ignore)
	return nil
```

- [ ] **Step 4: Allow memleak in the `[I]` guard in `internal/tui/doctor/update.go`**

The current guard in the `case "I":` branch of `handleListKey` is:

```go
			if f.Detector != "runaway" && f.Detector != "zombie" && f.Detector != "herd" {
				return m.adjustOffset(), nil
			}
```

Replace with:

```go
			if f.Detector != "runaway" && f.Detector != "zombie" && f.Detector != "herd" && f.Detector != "memleak" {
				return m.adjustOffset(), nil
			}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/config/ ./internal/tui/doctor/ -count=1`
Expected: PASS (new tests + all existing).

- [ ] **Step 6: Commit**

```bash
gofmt -w internal/config/ignore.go internal/tui/doctor/update.go internal/config/ignore_test.go internal/tui/doctor/pause_ignore_test.go
git add internal/config/ignore.go internal/tui/doctor/update.go internal/config/ignore_test.go internal/tui/doctor/pause_ignore_test.go
git commit -m "feat(doctor): allow ignoring memleak findings from the TUI"
```

---

## Task 4: Wire memleak into the engine

**Files:**
- Modify: `cmd/shrike/doctor.go` (`buildEngine`)

- [ ] **Step 1: Register the detector and its config in `buildEngine`**

The current detector slice is:

```go
	all := []core.Detector{detectors.NewRunaway(), detectors.NewZombie(), detectors.NewHerd()}
```

Replace with:

```go
	all := []core.Detector{detectors.NewRunaway(), detectors.NewZombie(), detectors.NewHerd(), detectors.NewMemleak()}
```

The current `configs` map ends with the `herd` entry:

```go
		"herd": {
			"min_size":            c.Herd.MinSize,
			"total_cpu_threshold": c.Herd.TotalCPUThreshold,
			"ignore":              c.Herd.Ignore,
		},
	}
```

Add a `memleak` entry (note `rss_threshold` is converted from MB to bytes as a `uint64`, matching the detector's `cfg["rss_threshold"].(uint64)` read):

```go
		"herd": {
			"min_size":            c.Herd.MinSize,
			"total_cpu_threshold": c.Herd.TotalCPUThreshold,
			"ignore":              c.Herd.Ignore,
		},
		"memleak": {
			"rss_threshold": uint64(c.Memleak.RSSThresholdMB) * 1024 * 1024,
			"min_age":       time.Duration(c.Memleak.MinAge),
			"ignore":        c.Memleak.Ignore,
		},
	}
```

- [ ] **Step 2: Build and run the whole suite with the race detector**

```
go build ./...
go test ./... -race -count=1
go vet ./...
```

Expected: `go build` clean; ALL packages pass; `go vet` silent.

- [ ] **Step 3: Non-interactive smoke checks**

```
go run ./cmd/shrike doctor --json
go run ./cmd/shrike doctor --only memleak --json
go run ./cmd/shrike doctor --help
```

`--json` and `--only memleak --json` must exit 0 (no findings) or 1 (findings) without panicking. `--help` prints usage. (Report which exit codes you saw.)

- [ ] **Step 4: Commit**

```bash
gofmt -w cmd/shrike/doctor.go
git add cmd/shrike/doctor.go
git commit -m "feat(doctor): register memleak detector in the engine"
```

---

## Task 5: Documentation

**Files:**
- Modify: `README.md` (Detectors + Usage), `CHANGELOG.md`

- [ ] **Step 1: README — Detectors section**

In `README.md`, in the `## Detectors` section (which lists runaway / zombie / herd), add a memleak entry matching the existing style, e.g.:

```markdown
- 🧠 **memleak** — processes using a lot of memory (RSS over a threshold), or whose memory grows steadily across scans (a likely leak). Growth detection needs several samples, so it is most effective with auto-refresh on or under `shrike watch`; a one-shot run only flags outright hogs.
```

- [ ] **Step 2: README — Usage `--only` line**

In `README.md`, find the `--only` usage line, currently:

```markdown
shrike doctor --only runaway   # run a specific detector (runaway | zombie | herd)
```

Replace the detector list with:

```markdown
shrike doctor --only runaway   # run a specific detector (runaway | zombie | herd | memleak)
```

- [ ] **Step 3: CHANGELOG — add to the Unreleased section**

In `CHANGELOG.md`, under the existing `## [Unreleased]` → `### Added` (created in feature A), add a bullet:

```markdown
- 🧠 `memleak` detector: flags processes over an RSS threshold (default 1 GB) and,
  across multiple scans, processes whose RSS grows steadily (a likely leak). Stateful,
  in-memory sampling — most effective with auto-refresh or `shrike watch`. A one-shot
  scan only flags outright memory hogs. Tunable via the `[memleak]` config section and
  ignorable from the TUI with `[I]`.
```

- [ ] **Step 4: Commit**

```bash
git add README.md CHANGELOG.md
git commit -m "docs: document the memleak detector"
```

- [ ] **Step 5: Final full verification**

Run: `go test ./... -race && go vet ./...`
Expected: all green, no vet output.

---

## Self-Review notes (resolved during planning)

- **Spec coverage:** detector type + hybrid rule + severity + state + reason (T1), config + defaults (T2), ignore.toml `section()` + merge + `[I]` guard synergy (T3), engine wiring + `--only` (T4), docs incl. one-shot caveat (T5). All spec sections mapped.
- **No wall-clock:** the detector uses only `ProcessInfo` fields (`RSS`, `StartedAt`, `ElapsedTime`) and sample ordering; tests feed snapshot sequences and are fully deterministic.
- **Stateful-detector safety:** documented on the type; the engine runs each `Detect` once per `Run` in a single goroutine, `Run`s sequential — no concurrent access to `hist`. `go test ./... -race` in T4 confirms.
- **Type consistency:** `Memleak`/`*Memleak`, `NewMemleak() *Memleak`, `pidHistory`, config key `rss_threshold` (uint64 bytes) read in T1 and written in T4, `MemleakConfig.RSSThresholdMB`/`MinAge`/`Ignore` used in T2/T3/T4 consistently. `formatBytes`/`memleakSeverity`/`memleakReason` defined and used only in T1.
- **Config key match:** detector reads `cfg["rss_threshold"].(uint64)`; `buildEngine` writes `uint64(... ) * 1024 * 1024`. Detector reads `cfg["min_age"].(time.Duration)`; `buildEngine` writes `time.Duration(c.Memleak.MinAge)`. Consistent with the runaway pattern.
