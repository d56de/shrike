# Pause/Resume + ignore-from-TUI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `[p]` pause/resume toggle and an `[I]` ignore-from-TUI hotkey to `shrike doctor`.

**Architecture:** Pause is a new `core.Action` (reusing the existing `Killer` interface) dispatched via a dedicated `[p]` branch in the doctor model; paused PIDs are pinned into the findings list as synthetic `"paused"` findings so they stay resumable. Ignore is model-handled (needs the finding's detector), shows its own `ModeConfirmIgnore` modal, and persists to a separate machine-managed `ignore.toml` that `config.Load()` merges into each detector's ignore list — detectors and the `Action` interface stay untouched.

**Tech Stack:** Go 1.26, Bubble Tea, BurntSushi/toml, standard `go test`.

**Spec:** `docs/superpowers/specs/2026-06-12-pause-resume-ignore-from-tui-design.md`

**Conventions for every commit:** run `gofmt`/`goimports`, then `go test ./...`. Commit messages use `feat:` / `test:` / `docs:`. No `Co-Authored-By` trailer (attribution disabled globally for this repo).

---

## Task 1: Pause action

**Files:**
- Create: `internal/actions/pause.go`
- Test: `internal/actions/pause_test.go`

Pause reuses `Killer`, `SystemKiller` (from `internal/actions/kill.go`) and the `fakeKiller` test helper (from `internal/actions/kill_test.go`) — same package, nothing to redefine.

- [ ] **Step 1: Write the failing tests**

Create `internal/actions/pause_test.go`:

```go
package actions

import (
	"context"
	"syscall"
	"testing"

	"github.com/d56de/shrike/internal/core"
)

func TestPause_StopsRunningProcess(t *testing.T) {
	k := &fakeKiller{}
	p := Pause{Killer: k}

	res := p.Execute(context.Background(), []core.ProcessInfo{{PID: 42, State: core.StateRunning}})

	if len(res) != 1 || res[0].Err != nil || res[0].Message != "paused" {
		t.Fatalf("expected paused, got %+v", res)
	}
	if len(k.sent) != 1 || k.sent[0] != syscall.SIGSTOP {
		t.Errorf("expected one SIGSTOP, got %+v", k.sent)
	}
}

func TestPause_ResumesStoppedProcess(t *testing.T) {
	k := &fakeKiller{}
	p := Pause{Killer: k}

	res := p.Execute(context.Background(), []core.ProcessInfo{{PID: 42, State: core.StateStopped}})

	if res[0].Message != "resumed" {
		t.Fatalf("expected resumed, got %+v", res)
	}
	if len(k.sent) != 1 || k.sent[0] != syscall.SIGCONT {
		t.Errorf("expected one SIGCONT, got %+v", k.sent)
	}
}

func TestPause_SkipsZombie(t *testing.T) {
	k := &fakeKiller{}
	p := Pause{Killer: k}

	res := p.Execute(context.Background(), []core.ProcessInfo{{PID: 42, State: core.StateZombie}})

	if res[0].Message != "skipped (zombie)" {
		t.Errorf("expected skip, got %+v", res)
	}
	if len(k.sent) != 0 {
		t.Errorf("expected no signal sent, got %+v", k.sent)
	}
}

func TestPause_ReportsPermissionError(t *testing.T) {
	k := &fakeKiller{notPermit: true}
	p := Pause{Killer: k}

	res := p.Execute(context.Background(), []core.ProcessInfo{{PID: 42, State: core.StateRunning}})

	if res[0].Err == nil {
		t.Error("expected error, got nil")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/actions/ -run TestPause -v`
Expected: FAIL — `undefined: Pause`.

- [ ] **Step 3: Write the implementation**

Create `internal/actions/pause.go`:

```go
package actions

import (
	"context"
	"syscall"

	"github.com/d56de/shrike/internal/core"
)

// Pause freezes a process with SIGSTOP or resumes it with SIGCONT, choosing
// per target from its State. It reuses the Killer interface from kill.go.
type Pause struct {
	Killer Killer
}

// NewPause returns a ready-to-use Pause action backed by a SystemKiller.
func NewPause() Pause { return Pause{Killer: SystemKiller{}} }

// Key implements core.Action.
func (Pause) Key() rune { return 'p' }

// Name implements core.Action.
func (Pause) Name() string { return "pause" }

// Confirm implements core.Action. Pause is reversible, so no confirmation.
func (Pause) Confirm() string { return "" }

// Destructive implements core.Action.
func (Pause) Destructive() bool { return false }

// Execute sends SIGCONT to stopped targets (resume) and SIGSTOP to everything
// else (pause), skipping zombies, which cannot be signalled meaningfully.
func (p Pause) Execute(_ context.Context, targets []core.ProcessInfo) []core.ActionResult {
	out := make([]core.ActionResult, 0, len(targets))
	for _, t := range targets {
		if t.State == core.StateZombie {
			out = append(out, core.ActionResult{PID: t.PID, Message: "skipped (zombie)"})
			continue
		}
		sig := syscall.SIGSTOP
		verb := "paused"
		if t.State == core.StateStopped {
			sig = syscall.SIGCONT
			verb = "resumed"
		}
		if err := p.Killer.Signal(t.PID, sig); err != nil {
			out = append(out, core.ActionResult{PID: t.PID, Err: err, Message: "not permitted"})
			continue
		}
		out = append(out, core.ActionResult{PID: t.PID, Message: verb})
	}
	return out
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/actions/ -run TestPause -v`
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
gofmt -w internal/actions/pause.go internal/actions/pause_test.go
git add internal/actions/pause.go internal/actions/pause_test.go
git commit -m "feat(actions): add pause/resume toggle action (SIGSTOP/SIGCONT)"
```

---

## Task 2: ignore.toml read/append module

**Files:**
- Create: `internal/config/ignore.go`
- Test: `internal/config/ignore_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/config/ignore_test.go`:

```go
package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppendIgnoreAt_CreatesFileAndDedups(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ignore.toml")

	if err := AppendIgnoreAt(path, "runaway", "node"); err != nil {
		t.Fatal(err)
	}
	// Second identical append is a no-op (idempotent).
	if err := AppendIgnoreAt(path, "runaway", "node"); err != nil {
		t.Fatal(err)
	}
	if err := AppendIgnoreAt(path, "zombie", "AdpSDKUtil"); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if !strings.Contains(s, "Managed by shrike") {
		t.Errorf("expected header comment, got:\n%s", s)
	}
	if strings.Count(s, "node") != 1 {
		t.Errorf("expected 'node' exactly once (deduped), got:\n%s", s)
	}
	if !strings.Contains(s, "AdpSDKUtil") {
		t.Errorf("expected zombie entry, got:\n%s", s)
	}
}

func TestAppendIgnoreAt_UnknownDetector(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ignore.toml")
	if err := AppendIgnoreAt(path, "bogus", "x"); err == nil {
		t.Error("expected error for unknown detector")
	}
}

func TestMergeIgnoresAt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ignore.toml")
	if err := AppendIgnoreAt(path, "runaway", "node"); err != nil {
		t.Fatal(err)
	}

	cfg := DefaultConfig() // runaway ignore already has WindowServer etc.
	if err := mergeIgnoresAt(path, &cfg); err != nil {
		t.Fatal(err)
	}

	found := false
	for _, c := range cfg.Runaway.Ignore {
		if c == "node" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'node' merged into runaway ignore, got %v", cfg.Runaway.Ignore)
	}
}

func TestMergeIgnoresAt_MissingFileIsNoop(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.toml")
	cfg := DefaultConfig()
	before := len(cfg.Runaway.Ignore)
	if err := mergeIgnoresAt(path, &cfg); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Runaway.Ignore) != before {
		t.Error("missing file should not change config")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/config/ -run 'Ignore' -v`
Expected: FAIL — `undefined: AppendIgnoreAt`.

- [ ] **Step 3: Write the implementation**

Create `internal/config/ignore.go`:

```go
package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/BurntSushi/toml"
)

// ignoreHeader is written at the top of every generated ignore.toml so a user
// who opens it understands it is machine-managed but safe to hand-edit.
const ignoreHeader = "# Managed by shrike — entries added via [I] in `shrike doctor`.\n" +
	"# Safe to edit or delete by hand.\n\n"

// sectionIgnore is one detector's ignore list inside ignore.toml.
type sectionIgnore struct {
	Ignore []string `toml:"ignore"`
}

// ignoreFileData mirrors the per-detector sections of config.toml, but carries
// only the machine-appended ignore lists.
type ignoreFileData struct {
	Runaway sectionIgnore `toml:"runaway"`
	Zombie  sectionIgnore `toml:"zombie"`
	Herd    sectionIgnore `toml:"herd"`
}

// section returns a pointer to the ignore slice for the named detector, or nil
// if the detector has no ignore section.
func (d *ignoreFileData) section(detector string) *[]string {
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
}

// IgnorePath returns the resolved path to ignore.toml, mirroring Path().
func IgnorePath() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "shrike", "ignore.toml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "shrike", "ignore.toml"), nil
}

// AppendIgnore adds command to the detector's ignore list in the default
// ignore.toml location. Idempotent.
func AppendIgnore(detector, command string) error {
	path, err := IgnorePath()
	if err != nil {
		return err
	}
	return AppendIgnoreAt(path, detector, command)
}

// AppendIgnoreAt adds command to the detector's ignore list in the ignore.toml
// at path, creating the file if needed. Idempotent — a command already present
// is a no-op.
func AppendIgnoreAt(path, detector, command string) error {
	data, err := loadIgnoreFile(path)
	if err != nil {
		return err
	}
	sec := data.section(detector)
	if sec == nil {
		return fmt.Errorf("unknown detector %q", detector)
	}
	if slices.Contains(*sec, command) {
		return nil
	}
	*sec = append(*sec, command)
	return writeIgnoreFile(path, data)
}

// loadIgnoreFile reads and decodes ignore.toml. A missing file yields an empty
// (non-nil) struct and no error.
func loadIgnoreFile(path string) (*ignoreFileData, error) {
	var d ignoreFileData
	raw, err := os.ReadFile(path) //nolint:gosec // path derived from XDG/home, not user input
	if errors.Is(err, os.ErrNotExist) {
		return &d, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read ignore file %s: %w", path, err)
	}
	if _, err := toml.Decode(string(raw), &d); err != nil {
		return nil, fmt.Errorf("decode ignore file %s: %w", path, err)
	}
	return &d, nil
}

// writeIgnoreFile serialises d to path with the managed header comment.
func writeIgnoreFile(path string, d *ignoreFileData) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	var buf bytes.Buffer
	buf.WriteString(ignoreHeader)
	if err := toml.NewEncoder(&buf).Encode(d); err != nil {
		return fmt.Errorf("encode ignore file: %w", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil { //nolint:gosec // user-owned config
		return fmt.Errorf("write ignore file %s: %w", path, err)
	}
	return nil
}

// mergeIgnoresAt loads ignore.toml at path and appends its entries (deduped)
// into the matching detector ignore slices of cfg.
func mergeIgnoresAt(path string, cfg *Config) error {
	d, err := loadIgnoreFile(path)
	if err != nil {
		return err
	}
	cfg.Runaway.Ignore = mergeDedup(cfg.Runaway.Ignore, d.Runaway.Ignore)
	cfg.Zombie.Ignore = mergeDedup(cfg.Zombie.Ignore, d.Zombie.Ignore)
	cfg.Herd.Ignore = mergeDedup(cfg.Herd.Ignore, d.Herd.Ignore)
	return nil
}

// mergeDedup appends each element of extra to base if not already present.
func mergeDedup(base, extra []string) []string {
	for _, e := range extra {
		if !slices.Contains(base, e) {
			base = append(base, e)
		}
	}
	return base
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/config/ -run 'Ignore' -v`
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
gofmt -w internal/config/ignore.go internal/config/ignore_test.go
git add internal/config/ignore.go internal/config/ignore_test.go
git commit -m "feat(config): add machine-managed ignore.toml read/append"
```

---

## Task 3: Merge ignore.toml in `config.Load`

**Files:**
- Modify: `internal/config/config.go` (the `Load` function, lines 102-121)
- Test: `internal/config/config_test.go` (add one test)

- [ ] **Step 1: Write the failing test**

Append to `internal/config/config_test.go`:

```go
func TestLoad_MergesIgnoreFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	ip, err := IgnorePath()
	if err != nil {
		t.Fatal(err)
	}
	if err := AppendIgnoreAt(ip, "runaway", "node"); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, c := range cfg.Runaway.Ignore {
		if c == "node" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected ignore.toml 'node' merged on Load, got %v", cfg.Runaway.Ignore)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -run TestLoad_MergesIgnoreFile -v`
Expected: FAIL — `node` not present (Load does not yet merge).

- [ ] **Step 3: Replace the `Load` function**

In `internal/config/config.go`, replace the existing `Load` function (lines 102-121) with:

```go
// Load reads the config file and applies defaults for missing sections/fields,
// then merges the machine-managed ignore.toml on top. Returns the default
// config (plus any ignores) if config.toml does not exist.
func Load() (Config, error) {
	path, err := Path()
	if err != nil {
		return Config{}, err
	}
	cfg := DefaultConfig()
	data, err := os.ReadFile(path) //nolint:gosec // path is derived from XDG_CONFIG_HOME or user home, not user input
	switch {
	case errors.Is(err, os.ErrNotExist):
		// No config.toml — keep defaults, still merge ignore.toml below.
	case err != nil:
		return Config{}, fmt.Errorf("read config %s: %w", path, err)
	default:
		if _, err := toml.Decode(string(data), &cfg); err != nil {
			return Config{}, fmt.Errorf("decode config %s: %w", path, err)
		}
	}
	// Merge machine-managed ignore.toml. Best-effort: a hand-corrupted ignore
	// file must never block the doctor from launching, so the error is dropped.
	if ip, ipErr := IgnorePath(); ipErr == nil {
		_ = mergeIgnoresAt(ip, &cfg)
	}
	return cfg, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/config/ -v`
Expected: PASS (all config tests, including the new merge test and the existing `TestLoad_*`).

- [ ] **Step 5: Commit**

```bash
gofmt -w internal/config/config.go internal/config/config_test.go
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): merge ignore.toml into Load()"
```

---

## Task 4: Model fields, mode, and helpers

**Files:**
- Modify: `internal/tui/doctor/model.go`

This task only adds compiling scaffolding (fields, one mode constant, helper methods). Behavior is wired in Tasks 5 and 6. Unused package-level methods are legal in Go, so the package still builds and existing tests pass.

- [ ] **Step 1: Add the new Mode constant**

In `internal/tui/doctor/model.go`, replace the Mode const block (lines 18-26) with:

```go
const (
	ModeList Mode = iota
	ModeConfirm
	ModeRunning
	ModeResults
	ModeInfo
	ModeSample
	ModeHelp
	ModeConfirmIgnore // ignore-from-TUI confirm modal
)
```

(Appended at the end so existing mode values do not renumber.)

- [ ] **Step 2: Add the `sort` import**

In `internal/tui/doctor/model.go`, change the import block (lines 4-12) to add `"sort"`:

```go
import (
	"fmt"
	"sort"
	"time"

	"github.com/d56de/shrike/internal/actions"
	"github.com/d56de/shrike/internal/core"
	"github.com/d56de/shrike/internal/history"
	"github.com/d56de/shrike/internal/tui/style"
)
```

- [ ] **Step 3: Add the new Model fields**

In `internal/tui/doctor/model.go`, inside the `Model` struct, immediately after the `Sampling`/`SamplePID`/`SampleStacks` fields (after line 93, before the closing `}` at line 94), add:

```go
	// Paused maps PIDs the user SIGSTOP'd this session to a snapshot of the
	// process (CPU zeroed). Paused processes are pinned into the findings list
	// as synthetic "paused" findings so they stay resumable even after the
	// detectors stop flagging them. Cleared per-PID on resume.
	Paused map[int]core.ProcessInfo

	// PauseAction performs SIGSTOP/SIGCONT. Injected so tests can stub it.
	PauseAction core.Action

	// IgnorePending is the finding awaiting an ignore-confirm
	// (Mode == ModeConfirmIgnore). Nil otherwise.
	IgnorePending *core.Finding

	// IgnorePath is the path to the machine-managed ignore.toml (injected by
	// the caller). Empty disables the [I] write.
	IgnorePath string
```

- [ ] **Step 4: Initialise `Paused` in the constructor**

In `internal/tui/doctor/model.go`, in `NewModelWithTheme` (lines 103-112), add `Paused` to the returned struct literal:

```go
func NewModelWithTheme(findings []core.Finding, acts []core.Action, theme style.Theme) Model {
	return Model{
		Findings: findings,
		Actions:  acts,
		Selected: map[int]bool{},
		Expanded: map[int]bool{},
		Paused:   map[int]core.ProcessInfo{},
		Mode:     ModeList,
		Theme:    theme,
	}
}
```

- [ ] **Step 5: Add the helper methods**

Append to `internal/tui/doctor/model.go`:

```go
// isPaused reports whether the PID is currently SIGSTOP'd by the user.
func (m Model) isPaused(pid int) bool {
	_, ok := m.Paused[pid]
	return ok
}

// pauseTargets resolves the processes a [p] press should signal: the selected
// findings (or the cursor finding), herds expanded to all members, zombie
// findings skipped (a dead/reaped process cannot be paused). De-duplicated by
// PID. State is NOT overridden here — the caller flips already-paused PIDs to
// StateStopped so Execute sends SIGCONT.
func (m Model) pauseTargets() []core.ProcessInfo {
	var targets []core.ProcessInfo
	collect := func(i int) {
		if i < 0 || i >= len(m.Findings) {
			return
		}
		f := m.Findings[i]
		if f.Detector == "zombie" {
			return
		}
		if f.Group != nil {
			targets = append(targets, f.Group.Parent)
			targets = append(targets, f.Group.Children...)
			return
		}
		targets = append(targets, f.Process)
	}
	if len(m.Selected) > 0 {
		for i := range m.Selected {
			collect(i)
		}
	} else {
		collect(m.Cursor)
	}
	seen := map[int]bool{}
	unique := targets[:0]
	for _, t := range targets {
		if seen[t.PID] {
			continue
		}
		seen[t.PID] = true
		unique = append(unique, t)
	}
	return unique
}

// mergePausedFindings appends a synthetic "paused" finding for every paused PID
// not already present in the findings list, so paused processes stay visible
// (and resumable) after detectors drop them. PIDs are sorted for a stable
// render order.
func (m Model) mergePausedFindings() Model {
	if len(m.Paused) == 0 {
		return m
	}
	present := map[int]bool{}
	for _, f := range m.Findings {
		present[f.Process.PID] = true
	}
	pids := make([]int, 0, len(m.Paused))
	for pid := range m.Paused {
		if !present[pid] {
			pids = append(pids, pid)
		}
	}
	sort.Ints(pids)
	for _, pid := range pids {
		m.Findings = append(m.Findings, core.Finding{
			Detector: "paused",
			Severity: core.SeverityLow,
			Process:  m.Paused[pid],
			Reason:   "paused by you",
		})
	}
	return m
}

// dropPausedFinding removes any synthetic "paused" finding for the given PID.
// Real findings for that PID (if any) are left in place.
func (m Model) dropPausedFinding(pid int) Model {
	out := m.Findings[:0]
	for _, f := range m.Findings {
		if f.Detector == "paused" && f.Process.PID == pid {
			continue
		}
		out = append(out, f)
	}
	m.Findings = out
	return m
}

// filterIgnored drops findings matching the just-ignored (detector, command)
// pair from the current view and re-clamps the cursor.
func (m Model) filterIgnored(detector, command string) Model {
	out := m.Findings[:0]
	for _, f := range m.Findings {
		if f.Detector == detector && f.Process.Command == command {
			continue
		}
		out = append(out, f)
	}
	m.Findings = out
	if m.Cursor >= len(m.Findings) {
		m.Cursor = len(m.Findings) - 1
	}
	if m.Cursor < 0 {
		m.Cursor = 0
	}
	return m
}
```

- [ ] **Step 6: Verify the package still builds and tests pass**

Run: `go build ./... && go test ./internal/tui/doctor/ -v`
Expected: PASS (existing doctor tests unaffected; new helpers compile unused).

- [ ] **Step 7: Commit**

```bash
gofmt -w internal/tui/doctor/model.go
git add internal/tui/doctor/model.go
git commit -m "feat(doctor): add paused/ignore model state and helpers"
```

---

## Task 5: Wire pause/resume into the update loop

**Files:**
- Modify: `internal/tui/doctor/update.go` (ActionMsg case, RescanDoneMsg case, add `[p]` branch)
- Test: `internal/tui/doctor/pause_ignore_test.go` (new)

- [ ] **Step 1: Write the failing tests**

Create `internal/tui/doctor/pause_ignore_test.go`:

```go
package doctor

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/d56de/shrike/internal/core"
)

// stubPause is an injectable Pause action that returns canned results without
// signalling any real process.
type stubPause struct {
	results []core.ActionResult
}

func (stubPause) Key() rune         { return 'p' }
func (stubPause) Name() string      { return "pause" }
func (stubPause) Confirm() string   { return "" }
func (stubPause) Destructive() bool { return false }
func (s stubPause) Execute(_ context.Context, _ []core.ProcessInfo) []core.ActionResult {
	return s.results
}

func keyRunes(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

func TestPause_OptimisticallyPinsThenResumes(t *testing.T) {
	pi := core.ProcessInfo{PID: 42, Command: "node", State: core.StateRunning, CPUPercent: 180}
	m := Model{
		Findings:    []core.Finding{{Detector: "runaway", Process: pi}},
		Selected:    map[int]bool{},
		Paused:      map[int]core.ProcessInfo{},
		KilledPIDs:  map[int]bool{},
		PauseAction: stubPause{results: []core.ActionResult{{PID: 42, Message: "paused"}}},
	}

	// [p] optimistically pins PID 42 and dispatches the action.
	mm, cmd := m.Update(keyRunes('p'))
	m = mm.(Model)
	if !m.isPaused(42) {
		t.Fatal("expected PID 42 optimistically pinned after [p]")
	}
	if cmd == nil {
		t.Fatal("expected a pause command")
	}
	// Deliver the resulting ActionMsg; PID stays paused.
	m2, _ := m.Update(cmd())
	m = m2.(Model)
	if !m.isPaused(42) {
		t.Error("expected PID 42 to remain paused after ActionMsg")
	}

	// Press [p] again → resume (PID already paused → SIGCONT path).
	m.PauseAction = stubPause{results: []core.ActionResult{{PID: 42, Message: "resumed"}}}
	mm, cmd = m.Update(keyRunes('p'))
	m = mm.(Model)
	m3, _ := m.Update(cmd())
	m = m3.(Model)
	if m.isPaused(42) {
		t.Error("expected PID 42 removed from Paused after resume")
	}
}

func TestPause_PinSurvivesRescan(t *testing.T) {
	m := Model{
		Selected:   map[int]bool{},
		KilledPIDs: map[int]bool{},
		Paused:     map[int]core.ProcessInfo{42: {PID: 42, Command: "node"}},
	}
	m2, _ := m.Update(RescanDoneMsg{Findings: []core.Finding{}, Err: nil})
	m = m2.(Model)

	found := false
	for _, f := range m.Findings {
		if f.Detector == "paused" && f.Process.PID == 42 {
			found = true
		}
	}
	if !found {
		t.Error("expected synthetic paused finding for PID 42 after rescan")
	}
}

func TestActionMsg_KillStillMarksKilled(t *testing.T) {
	m := Model{
		Selected:   map[int]bool{},
		KilledPIDs: map[int]bool{},
		Paused:     map[int]core.ProcessInfo{},
	}
	m2, _ := m.Update(ActionMsg{Results: []core.ActionResult{{PID: 99, Message: "sent SIGTERM"}}})
	m = m2.(Model)
	if !m.KilledPIDs[99] {
		t.Error("expected PID 99 marked killed (kill path regression)")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/tui/doctor/ -run 'TestPause|TestActionMsg_Kill' -v`
Expected: FAIL — `[p]` does not pin (no branch yet); `TestActionMsg_KillStillMarksKilled` may pass already but the others fail.

- [ ] **Step 3: Replace the ActionMsg case**

In `internal/tui/doctor/update.go`, replace the `case ActionMsg:` block (lines 80-99) with:

```go
	case ActionMsg:
		m.LastResults = msg.Results
		m.Mode = ModeResults
		m.ActionRunning = false
		if m.KilledPIDs == nil {
			m.KilledPIDs = map[int]bool{}
		}
		if m.Paused == nil {
			m.Paused = map[int]core.ProcessInfo{}
		}
		for _, r := range msg.Results {
			switch r.Message {
			case "paused":
				// Already optimistically pinned in the [p] branch.
			case "resumed":
				delete(m.Paused, r.PID)
				m = m.dropPausedFinding(r.PID)
			case "skipped (zombie)":
				// no-op
			default:
				if r.Err != nil {
					// Undo the optimistic pin on a failed pause/resume; a
					// failed kill/renice simply isn't marked killed (as before).
					delete(m.Paused, r.PID)
					continue
				}
				m.KilledPIDs[r.PID] = true
			}
		}
		// Drop the bulk selection now that the action is done.
		m.Selected = map[int]bool{}
		m = m.mergePausedFindings()
		if m.Cursor >= len(m.Findings) {
			m.Cursor = len(m.Findings) - 1
		}
		if m.Cursor < 0 {
			m.Cursor = 0
		}
		return m.adjustOffset(), nil
```

- [ ] **Step 4: Add the paused-finding re-pin to RescanDoneMsg**

In `internal/tui/doctor/update.go`, in the `case RescanDoneMsg:` block, after the line `m.KilledPIDs = nil` (line 133) and before `m.Cursor = findPIDIndex(...)` (line 134), insert:

```go
				// Re-pin paused rows the rescan no longer flags so they stay
				// resumable. Done before findPIDIndex so the cursor can land on
				// a re-pinned row.
				m = m.mergePausedFindings()
```

- [ ] **Step 5: Add the `[p]` branch to handleListKey**

In `internal/tui/doctor/update.go`, in `handleListKey`, insert a new `case "p":` immediately before the `default:` case (before line 289):

```go
		case "p":
			if m.PauseAction == nil {
				return m.adjustOffset(), nil
			}
			raw := m.pauseTargets()
			if len(raw) == 0 {
				return m.adjustOffset(), nil
			}
			if m.Paused == nil {
				m.Paused = map[int]core.ProcessInfo{}
			}
			targets := make([]core.ProcessInfo, 0, len(raw))
			for _, t := range raw {
				if _, paused := m.Paused[t.PID]; paused {
					t.State = core.StateStopped // → SIGCONT (resume)
				} else {
					pinned := t
					pinned.CPUPercent = 0
					m.Paused[t.PID] = pinned // optimistic pin
				}
				targets = append(targets, t)
			}
			return m, runAction(m.PauseAction, targets)
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/tui/doctor/ -run 'TestPause|TestActionMsg_Kill' -v`
Expected: PASS (3 tests).

- [ ] **Step 7: Run the full doctor + actions suites for regressions**

Run: `go test ./internal/tui/doctor/ ./internal/actions/ -race`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
gofmt -w internal/tui/doctor/update.go internal/tui/doctor/pause_ignore_test.go
git add internal/tui/doctor/update.go internal/tui/doctor/pause_ignore_test.go
git commit -m "feat(doctor): wire pause/resume toggle with session pinning"
```

---

## Task 6: Wire ignore-from-TUI into the update loop

**Files:**
- Modify: `internal/tui/doctor/update.go` (import config, clear IgnorePending on esc, route ModeConfirmIgnore, `[I]` branch, new handler)
- Test: `internal/tui/doctor/pause_ignore_test.go` (add tests)

- [ ] **Step 1: Write the failing tests**

Append to `internal/tui/doctor/pause_ignore_test.go`:

```go
func TestIgnore_WritesFileAndFilters(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ignore.toml")
	m := Model{
		Findings:   []core.Finding{{Detector: "runaway", Process: core.ProcessInfo{PID: 7, Command: "node"}}},
		Selected:   map[int]bool{},
		Paused:     map[int]core.ProcessInfo{},
		KilledPIDs: map[int]bool{},
		IgnorePath: path,
	}

	// [I] opens the ignore confirm.
	mm, _ := m.Update(keyRunes('I'))
	m = mm.(Model)
	if m.Mode != ModeConfirmIgnore || m.IgnorePending == nil {
		t.Fatalf("expected ModeConfirmIgnore with pending finding, got mode=%v pending=%v", m.Mode, m.IgnorePending)
	}

	// [y] confirms: writes the file and drops the matching finding.
	mm, _ = m.Update(keyRunes('y'))
	m = mm.(Model)
	if len(m.Findings) != 0 {
		t.Errorf("expected the node finding filtered out, got %d findings", len(m.Findings))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected ignore.toml written: %v", err)
	}
	if !strings.Contains(string(data), "node") {
		t.Errorf("expected 'node' in ignore.toml, got:\n%s", string(data))
	}
}

func TestIgnore_CancelLeavesNoFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ignore.toml")
	m := Model{
		Findings:   []core.Finding{{Detector: "runaway", Process: core.ProcessInfo{PID: 7, Command: "node"}}},
		Selected:   map[int]bool{},
		Paused:     map[int]core.ProcessInfo{},
		KilledPIDs: map[int]bool{},
		IgnorePath: path,
	}
	mm, _ := m.Update(keyRunes('I'))
	m = mm.(Model)
	mm, _ = m.Update(keyRunes('n'))
	m = mm.(Model)

	if m.Mode != ModeList || m.IgnorePending != nil {
		t.Errorf("expected back to list with no pending, got mode=%v pending=%v", m.Mode, m.IgnorePending)
	}
	if len(m.Findings) != 1 {
		t.Error("expected the finding to remain on cancel")
	}
	if _, err := os.Stat(path); err == nil {
		t.Error("expected no ignore.toml written on cancel")
	}
}

func TestIgnore_RefusesSyntheticPausedFinding(t *testing.T) {
	m := Model{
		Findings:   []core.Finding{{Detector: "paused", Process: core.ProcessInfo{PID: 7, Command: "node"}}},
		Selected:   map[int]bool{},
		Paused:     map[int]core.ProcessInfo{7: {PID: 7, Command: "node"}},
		KilledPIDs: map[int]bool{},
		IgnorePath: filepath.Join(t.TempDir(), "ignore.toml"),
	}
	mm, _ := m.Update(keyRunes('I'))
	m = mm.(Model)
	if m.Mode == ModeConfirmIgnore {
		t.Error("expected [I] to be a no-op on a synthetic paused finding")
	}
}
```

Add the imports this file now needs — update the import block at the top of `internal/tui/doctor/pause_ignore_test.go` to:

```go
import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/d56de/shrike/internal/core"
)
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/tui/doctor/ -run TestIgnore -v`
Expected: FAIL — `[I]` does not enter ModeConfirmIgnore (no branch yet).

- [ ] **Step 3: Add the config import to update.go**

In `internal/tui/doctor/update.go`, change the import block (lines 3-12) to add the config package:

```go
import (
	"context"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/d56de/shrike/internal/actions"
	"github.com/d56de/shrike/internal/config"
	"github.com/d56de/shrike/internal/core"
	"github.com/d56de/shrike/internal/history"
)
```

- [ ] **Step 4: Clear IgnorePending on the global esc, and route ModeConfirmIgnore**

In `internal/tui/doctor/update.go`, in `handleKey`, replace the `case "esc":` block (lines 180-182) with:

```go
	case "esc":
		m.Mode = ModeList
		m.IgnorePending = nil
		return m, nil
	}
```

Then, in the `switch m.Mode` block in `handleKey`, add a case for the new mode (after the `case ModeConfirm:` line 188-189):

```go
	case ModeConfirmIgnore:
		return m.handleIgnoreConfirmKey(msg)
```

- [ ] **Step 5: Add the `[I]` branch to handleListKey**

In `internal/tui/doctor/update.go`, in `handleListKey`, insert a new `case "I":` immediately before the `default:` case (and before the `case "p":` added in Task 5, order does not matter):

```go
		case "I":
			if m.Cursor < 0 || m.Cursor >= len(m.Findings) {
				return m.adjustOffset(), nil
			}
			f := m.Findings[m.Cursor]
			// Only the three real detectors have ignore lists; synthetic
			// "paused" pins cannot be ignored.
			if f.Detector != "runaway" && f.Detector != "zombie" && f.Detector != "herd" {
				return m.adjustOffset(), nil
			}
			pending := f
			m.IgnorePending = &pending
			m.Mode = ModeConfirmIgnore
			return m, nil
```

- [ ] **Step 6: Add the ignore-confirm key handler**

In `internal/tui/doctor/update.go`, add a new function right after `handleConfirmKey` (after line 346):

```go
// handleIgnoreConfirmKey handles input while the ignore-confirm modal is open.
func (m Model) handleIgnoreConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "enter":
		if m.IgnorePending == nil {
			m.Mode = ModeList
			return m, nil
		}
		f := *m.IgnorePending
		if err := config.AppendIgnoreAt(m.IgnorePath, f.Detector, f.Process.Command); err != nil {
			m.LastResults = []core.ActionResult{{
				PID: f.Process.PID, Err: err, Message: "ignore failed: " + err.Error(),
			}}
			m.IgnorePending = nil
			m.Mode = ModeResults
			return m, nil
		}
		m = m.filterIgnored(f.Detector, f.Process.Command)
		m.IgnorePending = nil
		m.Mode = ModeList
		return m.adjustOffset(), nil
	case "n":
		m.IgnorePending = nil
		m.Mode = ModeList
		return m, nil
	}
	return m, nil
}
```

- [ ] **Step 7: Run tests to verify they pass**

Run: `go test ./internal/tui/doctor/ -run TestIgnore -v`
Expected: PASS (3 tests).

- [ ] **Step 8: Commit**

```bash
gofmt -w internal/tui/doctor/update.go internal/tui/doctor/pause_ignore_test.go
git add internal/tui/doctor/update.go internal/tui/doctor/pause_ignore_test.go
git commit -m "feat(doctor): wire ignore-from-TUI with confirm modal"
```

---

## Task 7: View — paused tag, ignore modal, footer/help

**Files:**
- Modify: `internal/tui/doctor/view.go`

- [ ] **Step 1: Add the ModeConfirmIgnore case to View**

In `internal/tui/doctor/view.go`, in the `switch m.Mode` inside `View()`, add a case after the `case ModeConfirm:` block (after line 49):

```go
	case ModeConfirmIgnore:
		title = "Shrike — ignore"
		body = renderIgnoreConfirmBody(t, m, innerWidth)
```

- [ ] **Step 2: Add the ignore-confirm renderer**

In `internal/tui/doctor/view.go`, add this function immediately after `renderConfirmBody` (after line 576):

```go
// renderIgnoreConfirmBody renders the ignore-from-TUI confirmation modal.
func renderIgnoreConfirmBody(t style.Theme, m Model, innerWidth int) string {
	f := m.IgnorePending
	if f == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(pad("") + "\n")
	q := fmt.Sprintf("Ignore all '%s' processes in the %s detector?",
		f.Process.Command, f.Detector)
	b.WriteString(pad(t.Accent.Render("⏺")+"  "+q) + "\n")
	b.WriteString(pad(t.Gutter.Render("│")) + "\n")
	b.WriteString(pad(t.Subtle.Render("  Saved to ignore.toml — they won't be flagged again.")) + "\n")
	b.WriteString(pad(t.Subtle.Render("  Edit or delete that file by hand to undo.")) + "\n")
	b.WriteString(pad("") + "\n")
	b.WriteString(footerDivider(t, innerWidth))
	b.WriteString(pad("") + "\n")
	b.WriteString(pad(t.KeyHint.Render("[y] confirm · [n] cancel")) + "\n")
	return b.String()
}
```

- [ ] **Step 3: Add the paused tag to list rows**

In `internal/tui/doctor/view.go`, in `renderListBody`, immediately after the `data := fmt.Sprintf(...)` assignment (ends at line 254) and before `if killed {` (line 255), insert:

```go
		// Pinned paused rows get a marker; a killed row keeps its strikethrough.
		if m.isPaused(f.Process.PID) && !killed {
			data = data + "  " + t.Accent.Render("⏸ paused")
		}
```

- [ ] **Step 4: Add `[p]` and `[I]` to the footer keyhints**

In `internal/tui/doctor/view.go`, in `keyhintSegments` (lines 324-336), replace the `segs = append(segs, ... "[R] rescan")` call (lines 329-331) with:

```go
	segs = append(segs,
		"[i]nfo", "[s]ample", "[k]ill", "[r]enice", "[p]ause", "[I]gnore",
		"[R] rescan")
```

- [ ] **Step 5: Add `p` and `I` to the help modal**

In `internal/tui/doctor/view.go`, in `renderHelpBody` (lines 798-813), add two rows to the `items` slice, after the `{"r", "renice +10"}` line (line 808):

```go
		{"p", "pause / resume (toggle SIGSTOP/SIGCONT)"},
		{"I", "ignore process (saved to ignore.toml)"},
```

- [ ] **Step 6: Write a light render test**

Append to `internal/tui/doctor/pause_ignore_test.go`:

```go
func TestView_RendersPausedTagAndHints(t *testing.T) {
	m := Model{
		Width:      120,
		Height:     40,
		Findings:   []core.Finding{{Detector: "paused", Process: core.ProcessInfo{PID: 42, Command: "node"}}},
		Selected:   map[int]bool{},
		Paused:     map[int]core.ProcessInfo{42: {PID: 42, Command: "node"}},
		KilledPIDs: map[int]bool{},
	}
	out := m.View()
	if !strings.Contains(out, "paused") {
		t.Error("expected '⏸ paused' marker in list output")
	}
	if !strings.Contains(out, "[p]ause") || !strings.Contains(out, "[I]gnore") {
		t.Error("expected [p]ause and [I]gnore in footer hints")
	}
}
```

- [ ] **Step 7: Run tests to verify they pass**

Run: `go test ./internal/tui/doctor/ -run 'TestView_RendersPausedTag' -v`
Expected: PASS.

- [ ] **Step 8: Run the full doctor suite for layout regressions**

Run: `go test ./internal/tui/doctor/ -race`
Expected: PASS (scroll / confirm-scroll / autorefresh tests still green).

- [ ] **Step 9: Commit**

```bash
gofmt -w internal/tui/doctor/view.go internal/tui/doctor/pause_ignore_test.go
git add internal/tui/doctor/view.go internal/tui/doctor/pause_ignore_test.go
git commit -m "feat(doctor): render paused tag, ignore modal, and [p]/[I] hints"
```

---

## Task 8: Wire into the doctor command + full verification

**Files:**
- Modify: `cmd/shrike/doctor.go`

- [ ] **Step 1: Inject the pause action and ignore path**

In `cmd/shrike/doctor.go`, in the interactive TUI path, immediately after the existing model setup (after the `model.AutoRefreshOn = ...` line and before `prog := tea.NewProgram(...)`), add:

```go
		model.PauseAction = actions_.NewPause()
		if ip, ipErr := cfg.IgnorePath(); ipErr == nil {
			model.IgnorePath = ip
		}
```

(`actions_` and `cfg` are the existing import aliases in this file for `internal/actions` and `internal/config`.)

- [ ] **Step 2: Build and run the whole suite with the race detector**

Run: `go build ./... && go test ./... -race`
Expected: PASS across all packages.

- [ ] **Step 3: Vet**

Run: `go vet ./...`
Expected: no output.

- [ ] **Step 4: Manual smoke test**

Run: `go run ./cmd/shrike doctor`
Expected: the footer shows `[p]ause` and `[I]gnore`; `?` help lists `p` and `I`. (If no findings are present, lower the threshold: `go run ./cmd/shrike doctor --threshold 1`.) Press `p` on a finding → row gains `⏸ paused`; press `p` again → marker clears. Press `I` → confirm modal; `y` removes the row and writes `~/.config/shrike/ignore.toml`. Press `q` to quit.

- [ ] **Step 5: Commit**

```bash
gofmt -w cmd/shrike/doctor.go
git add cmd/shrike/doctor.go
git commit -m "feat(doctor): enable pause action and ignore path in doctor cmd"
```

---

## Task 9: Documentation

**Files:**
- Modify: `README.md` (Actions section), `CHANGELOG.md`

- [ ] **Step 1: Update the README Actions list**

In `README.md`, in the `## Actions` section, add two bullets after the `[r]` renice line:

```markdown
- `[p]` pause / resume — SIGSTOP to freeze a runaway, SIGCONT to thaw it; paused rows stay pinned (`⏸ paused`) so they remain resumable
- `[I]` ignore — add the process to the detector's ignore list (saved to `ignore.toml`); it won't be flagged again
```

- [ ] **Step 2: Mention ignore.toml near the config docs**

In `README.md`, in the `## Usage` section (or wherever `config edit` is documented), add a one-line note:

```markdown
Ignores added with `[I]` in the TUI are stored in `ignore.toml` next to `config.toml` and merged on load — your hand-written `config.toml` is never rewritten.
```

- [ ] **Step 3: Add a CHANGELOG entry**

In `CHANGELOG.md`, add a new section directly under the `# Changelog` header / intro, above `## [v0.3.0]`:

```markdown
## [Unreleased]

### Added

- `[p]` pause/resume action in `shrike doctor`: SIGSTOP freezes a runaway
  process, SIGCONT thaws it. Paused processes are pinned into the list
  (`⏸ paused`) so they stay resumable even after detectors stop flagging them.
- `[I]` ignore-from-TUI: add the selected process to its detector's ignore list
  without leaving the TUI. Entries persist to a separate machine-managed
  `ignore.toml`, merged into config on load, so the hand-written `config.toml`
  is never rewritten.
```

- [ ] **Step 4: Commit**

```bash
git add README.md CHANGELOG.md
git commit -m "docs: document [p] pause/resume and [I] ignore-from-TUI"
```

- [ ] **Step 5: Final full verification**

Run: `go test ./... -race && go vet ./...`
Expected: all green, no vet output.

---

## Self-Review notes (resolved during planning)

- **Spec coverage:** Pause action (T1), ignore.toml module (T2) + Load merge (T3), model state/helpers (T4), pause wiring + session pinning (T5), ignore wiring + confirm (T6), view tag/modal/hints (T7), command wiring (T8), docs (T9). All spec sections mapped.
- **Toggle correctness:** the STOP-vs-CONT decision is driven by the model's `Paused` set (source of truth), not the stale `ProcessInfo.State` — the `[p]` branch flips already-paused targets to `StateStopped` before calling Execute. Documented in T5 Step 5.
- **Spec refinement:** the spec said "reuse `ModeConfirm` with an `IgnorePending` branch"; the plan uses a dedicated `ModeConfirmIgnore` mode instead. This is strictly safer (no stale-pending cross-render between the action-confirm and ignore-confirm renderers) and matches the codebase's existing per-mode handler pattern. Behavior is identical to the approved spec.
- **Kill/renice regression guard:** the rewritten ActionMsg handler routes only pause/resume messages specially; all other results fall through to the existing `KilledPIDs` behavior (covered by `TestActionMsg_KillStillMarksKilled`).
- **Type consistency:** `Paused map[int]core.ProcessInfo`, `PauseAction core.Action`, `IgnorePending *core.Finding`, `IgnorePath string`, `ModeConfirmIgnore`, and methods `isPaused`/`pauseTargets`/`mergePausedFindings`/`dropPausedFinding`/`filterIgnored` are used consistently across Tasks 4–8.
