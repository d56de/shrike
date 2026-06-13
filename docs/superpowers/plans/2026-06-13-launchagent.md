# `shrike watch` LaunchAgent Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `shrike watch --install` / `--uninstall` / `--status` to register the watch loop as a per-user macOS LaunchAgent that starts at login.

**Architecture:** A new `internal/agent` package owns plist generation (a pure function) and the `launchctl` lifecycle behind an injected runner. `cmd/shrike/watch.go` gains three mutually-exclusive management flags that dispatch to the package; with no flag, `watch` runs the existing foreground loop unchanged.

**Tech Stack:** Go 1.26, cobra, `os/exec` (launchctl), standard `go test`.

**Spec:** `docs/superpowers/specs/2026-06-13-launchagent-design.md`

**Conventions for every commit:** run `gofmt`/`goimports`, then `go test ./...`. Commit messages use `feat:` / `test:` / `docs:`. No `Co-Authored-By` trailer (attribution disabled globally for this repo).

**SAFETY:** No automated step may run `shrike watch --install` on its own — that installs a real LaunchAgent on the machine. Automated tests use the injected fake `launchctl`; the only real launchctl invocation allowed is the read-only `shrike watch --status`.

---

## Task 1: `internal/agent` package

**Files:**
- Create: `internal/agent/agent.go`
- Test: `internal/agent/agent_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/agent/agent_test.go`:

```go
package agent

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlist_Contents(t *testing.T) {
	out := Plist("de.d56.shrike.watch", "/opt/homebrew/bin/shrike", "/log/shrike.log")
	checks := []string{
		"<string>de.d56.shrike.watch</string>",
		"<string>/opt/homebrew/bin/shrike</string>",
		"<string>watch</string>",
		"<string>--quiet</string>",
		"<key>RunAtLoad</key>",
		"<key>KeepAlive</key>",
		"<true/>",
		"/opt/homebrew/bin:/usr/local/bin",
	}
	for _, c := range checks {
		if !strings.Contains(out, c) {
			t.Errorf("plist missing %q in:\n%s", c, out)
		}
	}
	if n := strings.Count(out, "/log/shrike.log"); n != 2 {
		t.Errorf("expected log path twice (stdout+stderr), got %d", n)
	}
}

func TestPlist_XMLEscapesPaths(t *testing.T) {
	out := Plist("L", "/path/a&b/shrike", "/log.log")
	if !strings.Contains(out, "a&amp;b") {
		t.Errorf("expected & escaped in exec path, got:\n%s", out)
	}
	if strings.Contains(out, "a&b/shrike") {
		t.Error("raw unescaped & should not appear")
	}
}

type recordRun struct {
	calls [][]string
	err   error
}

func (r *recordRun) run(args ...string) error {
	r.calls = append(r.calls, args)
	return r.err
}

func newTestManager(plistPath string, rr *recordRun) *Manager {
	return &Manager{
		Label:     "de.d56.shrike.watch",
		PlistPath: plistPath,
		ExecPath:  "/bin/shrike",
		LogPath:   "/log/shrike.log",
		UID:       501,
		run:       rr.run,
	}
}

func TestInstall_WritesPlistAndBootstraps(t *testing.T) {
	dir := t.TempDir()
	plistPath := filepath.Join(dir, "LaunchAgents", "de.d56.shrike.watch.plist")
	rr := &recordRun{}
	m := newTestManager(plistPath, rr)

	if err := m.Install(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(plistPath)
	if err != nil {
		t.Fatalf("plist not written: %v", err)
	}
	if !strings.Contains(string(data), "<string>/bin/shrike</string>") {
		t.Errorf("plist content wrong:\n%s", data)
	}
	if len(rr.calls) != 2 {
		t.Fatalf("expected 2 launchctl calls (bootout, bootstrap), got %d: %v", len(rr.calls), rr.calls)
	}
	if rr.calls[0][0] != "bootout" || rr.calls[0][1] != "gui/501/de.d56.shrike.watch" {
		t.Errorf("call[0] = %v, want [bootout gui/501/de.d56.shrike.watch]", rr.calls[0])
	}
	if rr.calls[1][0] != "bootstrap" || rr.calls[1][1] != "gui/501" || rr.calls[1][2] != plistPath {
		t.Errorf("call[1] = %v, want [bootstrap gui/501 %s]", rr.calls[1], plistPath)
	}
}

func TestUninstall_BootoutAndRemove(t *testing.T) {
	dir := t.TempDir()
	plistPath := filepath.Join(dir, "de.d56.shrike.watch.plist")
	if err := os.WriteFile(plistPath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	rr := &recordRun{}
	m := newTestManager(plistPath, rr)

	if err := m.Uninstall(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(plistPath); !os.IsNotExist(err) {
		t.Error("expected plist removed")
	}
	if len(rr.calls) != 1 || rr.calls[0][0] != "bootout" {
		t.Errorf("expected one bootout call, got %v", rr.calls)
	}
}

func TestUninstall_NoPlistIsOK(t *testing.T) {
	rr := &recordRun{}
	m := newTestManager(filepath.Join(t.TempDir(), "missing.plist"), rr)
	if err := m.Uninstall(); err != nil {
		t.Errorf("uninstall with no plist should succeed, got %v", err)
	}
}

func TestLoaded(t *testing.T) {
	m := newTestManager("/x.plist", &recordRun{}) // run returns nil → loaded
	if ok, _ := m.Loaded(); !ok {
		t.Error("expected loaded when launchctl print succeeds")
	}
	m2 := newTestManager("/x.plist", &recordRun{err: errors.New("not found")})
	if ok, _ := m2.Loaded(); ok {
		t.Error("expected not-loaded when launchctl print fails")
	}
}
```

- [ ] **Step 2: Run tests, verify FAIL (undefined: Plist / Manager)**

Run: `go test ./internal/agent/ -v`
Expected: FAIL (package/symbols don't exist).

- [ ] **Step 3: Create `internal/agent/agent.go`**

```go
// Package agent manages the shrike LaunchAgent — a per-user macOS launchd job
// that runs `shrike watch` in the background.
package agent

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Label is the LaunchAgent's launchd label (reverse-DNS of the d56.de domain).
const Label = "de.d56.shrike.watch"

// agentPATH is the PATH given to the agent so it can find Homebrew-installed
// helpers (e.g. terminal-notifier); launchd otherwise supplies a minimal PATH.
const agentPATH = "/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"

const plistTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>watch</string>
		<string>--quiet</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
	<key>StandardOutPath</key>
	<string>%s</string>
	<key>StandardErrorPath</key>
	<string>%s</string>
	<key>EnvironmentVariables</key>
	<dict>
		<key>PATH</key>
		<string>%s</string>
	</dict>
</dict>
</plist>
`

// Plist generates the LaunchAgent plist XML for the given label, shrike binary
// path, and log path. Path/label values are XML-escaped.
func Plist(label, execPath, logPath string) string {
	return fmt.Sprintf(plistTemplate,
		xmlEscape(label), xmlEscape(execPath), xmlEscape(logPath), xmlEscape(logPath), agentPATH)
}

func xmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(s)
}

// Manager manages the LaunchAgent lifecycle via launchctl.
type Manager struct {
	Label     string
	PlistPath string // ~/Library/LaunchAgents/<label>.plist
	ExecPath  string // absolute path to the shrike binary
	LogPath   string // StandardOut/ErrPath target
	UID       int
	run       func(args ...string) error // launchctl; injected
}

// NewManager builds a Manager for the given shrike binary and log paths,
// resolving the plist location (~/Library/LaunchAgents) and the current UID.
func NewManager(execPath, logPath string) (*Manager, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home dir: %w", err)
	}
	return &Manager{
		Label:     Label,
		PlistPath: filepath.Join(home, "Library", "LaunchAgents", Label+".plist"),
		ExecPath:  execPath,
		LogPath:   logPath,
		UID:       os.Getuid(),
		run: func(args ...string) error {
			// nil Stdout/Stderr → launchctl output is discarded; only the exit
			// status matters.
			return exec.Command("launchctl", args...).Run() //nolint:gosec // fixed command, controlled args
		},
	}, nil
}

func (m *Manager) guiDomain() string     { return fmt.Sprintf("gui/%d", m.UID) }
func (m *Manager) serviceTarget() string { return fmt.Sprintf("gui/%d/%s", m.UID, m.Label) }

func (m *Manager) plist() string { return Plist(m.Label, m.ExecPath, m.LogPath) }

// Install writes the plist and (re)loads it. Idempotent: an already-loaded
// agent is booted out first, then bootstrapped.
func (m *Manager) Install() error {
	if err := os.MkdirAll(filepath.Dir(m.PlistPath), 0o755); err != nil {
		return fmt.Errorf("create LaunchAgents dir: %w", err)
	}
	if err := os.WriteFile(m.PlistPath, []byte(m.plist()), 0o644); err != nil { //nolint:gosec // user-owned config
		return fmt.Errorf("write plist %s: %w", m.PlistPath, err)
	}
	_ = m.run("bootout", m.serviceTarget()) // best-effort: ignore "not loaded"
	if err := m.run("bootstrap", m.guiDomain(), m.PlistPath); err != nil {
		return fmt.Errorf("launchctl bootstrap: %w", err)
	}
	return nil
}

// Uninstall boots out the agent (best-effort) and removes the plist file.
func (m *Manager) Uninstall() error {
	_ = m.run("bootout", m.serviceTarget()) // best-effort
	if err := os.Remove(m.PlistPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove plist %s: %w", m.PlistPath, err)
	}
	return nil
}

// Loaded reports whether the agent is currently bootstrapped.
func (m *Manager) Loaded() (bool, error) {
	if err := m.run("print", m.serviceTarget()); err != nil {
		return false, nil
	}
	return true, nil
}
```

- [ ] **Step 4: Run tests, verify pass**

Run: `go test ./internal/agent/ -v` (6 tests pass)

- [ ] **Step 5: Vet + commit**

```bash
gofmt -w internal/agent/agent.go internal/agent/agent_test.go
go vet ./internal/agent/
git add internal/agent/agent.go internal/agent/agent_test.go
git commit -m "feat(agent): add LaunchAgent plist generation + launchctl manager"
```

---

## Task 2: `--install` / `--uninstall` / `--status` flags on `watch`

**Files:**
- Modify: `cmd/shrike/watch.go`
- Test: `cmd/shrike/watch_test.go` (append)

The current `watch.go` declares its flag vars in a `var (...)` block (`watchInterval`, `watchLevel`, `watchQuiet`), registers them in `init()`, and its `RunE` begins with `c, err := cfg.Load()`. It already imports `fmt`, `os`, `history`, and `cobra`.

- [ ] **Step 1: Append the failing tests to `cmd/shrike/watch_test.go`**

```go
func TestHandleAgentFlags_MutualExclusion(t *testing.T) {
	watchInstall, watchUninstall, watchStatus = true, true, false
	defer func() { watchInstall, watchUninstall, watchStatus = false, false, false }()
	handled, err := handleAgentFlags(watchCmd)
	if !handled || err == nil {
		t.Errorf("expected (handled=true, err!=nil) for conflicting flags, got (%v, %v)", handled, err)
	}
}

func TestHandleAgentFlags_NoneSetIsUnhandled(t *testing.T) {
	watchInstall, watchUninstall, watchStatus = false, false, false
	handled, err := handleAgentFlags(watchCmd)
	if handled || err != nil {
		t.Errorf("expected (false, nil) when no agent flag set, got (%v, %v)", handled, err)
	}
}
```

- [ ] **Step 2: Run tests, verify FAIL (undefined: watchInstall / handleAgentFlags)**

Run: `go test ./cmd/shrike/ -run TestHandleAgentFlags -v`

- [ ] **Step 3: Add the management flag vars in `cmd/shrike/watch.go`**

The current var block is:

```go
var (
	watchInterval time.Duration
	watchLevel    string
	watchQuiet    bool
)
```

Replace with:

```go
var (
	watchInterval  time.Duration
	watchLevel     string
	watchQuiet     bool
	watchInstall   bool
	watchUninstall bool
	watchStatus    bool
)
```

- [ ] **Step 4: Add the `agent` import**

Add to `cmd/shrike/watch.go`'s import block:

```go
	"github.com/d56de/shrike/internal/agent"
```

- [ ] **Step 5: Call the management branch at the top of `RunE`**

`RunE` currently begins:

```go
	RunE: func(cmd *cobra.Command, _ []string) error {
		c, err := cfg.Load()
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
```

Insert the agent-mode check as the very first thing in `RunE` (before `c, err := cfg.Load()`):

```go
	RunE: func(cmd *cobra.Command, _ []string) error {
		if handled, err := handleAgentFlags(cmd); handled || err != nil {
			return err
		}

		c, err := cfg.Load()
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
```

- [ ] **Step 6: Add the `handleAgentFlags` function**

Add this function to `cmd/shrike/watch.go` (e.g. after `parseLevel`):

```go
// handleAgentFlags processes the --install/--uninstall/--status management
// flags. It returns handled=true when one was set (so the foreground loop is
// skipped), along with the action's error. With no flag set it returns
// (false, nil) so the caller runs the watch loop.
func handleAgentFlags(cmd *cobra.Command) (bool, error) {
	n := 0
	for _, b := range []bool{watchInstall, watchUninstall, watchStatus} {
		if b {
			n++
		}
	}
	if n == 0 {
		return false, nil
	}
	if n > 1 {
		return true, fmt.Errorf("--install, --uninstall, and --status are mutually exclusive")
	}

	exe, err := os.Executable()
	if err != nil {
		return true, fmt.Errorf("resolve shrike binary path: %w", err)
	}
	logPath, err := history.LogPath()
	if err != nil {
		return true, err
	}
	mgr, err := agent.NewManager(exe, logPath)
	if err != nil {
		return true, err
	}
	out := cmd.OutOrStdout()

	switch {
	case watchInstall:
		if err := mgr.Install(); err != nil {
			return true, err
		}
		_, _ = fmt.Fprintf(out, "✓ shrike watch installed as a LaunchAgent — starts at login.\n  Logs: %s\n", mgr.LogPath)
	case watchUninstall:
		if err := mgr.Uninstall(); err != nil {
			return true, err
		}
		_, _ = fmt.Fprintln(out, "✓ shrike watch LaunchAgent removed")
	case watchStatus:
		loaded, err := mgr.Loaded()
		if err != nil {
			return true, err
		}
		if loaded {
			_, _ = fmt.Fprintf(out, "running (LaunchAgent loaded)\n  plist: %s\n", mgr.PlistPath)
		} else {
			_, _ = fmt.Fprintln(out, "not installed\n  (run `shrike watch --install` to run in the background)")
		}
	}
	return true, nil
}
```

- [ ] **Step 7: Register the flags in `init()`**

The current `init()` is:

```go
func init() {
	watchCmd.Flags().DurationVar(&watchInterval, "interval", 60*time.Second, "scan interval (overrides config)")
	watchCmd.Flags().StringVar(&watchLevel, "notify-level", "high", "minimum severity to notify: low|medium|high|critical")
	watchCmd.Flags().BoolVar(&watchQuiet, "quiet", false, "suppress the per-scan status line")
}
```

Add the three management flags:

```go
func init() {
	watchCmd.Flags().DurationVar(&watchInterval, "interval", 60*time.Second, "scan interval (overrides config)")
	watchCmd.Flags().StringVar(&watchLevel, "notify-level", "high", "minimum severity to notify: low|medium|high|critical")
	watchCmd.Flags().BoolVar(&watchQuiet, "quiet", false, "suppress the per-scan status line")
	watchCmd.Flags().BoolVar(&watchInstall, "install", false, "install shrike watch as a background LaunchAgent (starts at login)")
	watchCmd.Flags().BoolVar(&watchUninstall, "uninstall", false, "remove the shrike watch LaunchAgent")
	watchCmd.Flags().BoolVar(&watchStatus, "status", false, "show whether the LaunchAgent is installed")
}
```

- [ ] **Step 8: Run the tests, verify pass**

Run: `go test ./cmd/shrike/ -run TestHandleAgentFlags -v` (2 tests pass) and `go test ./cmd/shrike/ -count=1` (existing watch tests stay green).

- [ ] **Step 9: Commit**

```bash
gofmt -w cmd/shrike/watch.go cmd/shrike/watch_test.go
go vet ./cmd/shrike/
git add cmd/shrike/watch.go cmd/shrike/watch_test.go
git commit -m "feat(cmd): shrike watch --install/--uninstall/--status (LaunchAgent)"
```

---

## Task 3: Verification + docs

**Files:**
- Modify: `README.md`, `CHANGELOG.md`

- [ ] **Step 1: Build + full race suite + vet**

```
go build ./...
go test ./... -race -count=1
go vet ./...
```

Expected: build clean; all packages pass under `-race`; `go vet` silent.

- [ ] **Step 2: Non-interactive, non-destructive smoke checks**

```
go run ./cmd/shrike watch --help
go run ./cmd/shrike watch --install --uninstall
go run ./cmd/shrike watch --status
```

- `watch --help` lists `--install`, `--uninstall`, `--status`; exit 0.
- `watch --install --uninstall` prints the mutual-exclusion error; exit non-zero.
- `watch --status` invokes real (read-only) `launchctl print`; on a machine without the agent it prints `not installed`; exit 0.

**DO NOT run `go run ./cmd/shrike watch --install` by itself** — it installs a real LaunchAgent. Report the three exit codes above.

- [ ] **Step 3: README — document the LaunchAgent flags**

In `README.md`, near the `shrike watch` usage lines, add:

```markdown
shrike watch --install         # run watch in the background as a LaunchAgent (starts at login)
shrike watch --status          # is the background agent installed?
shrike watch --uninstall       # remove the background agent
```

And a one-line note (near the Watch prose):

```markdown
`shrike watch --install` registers a per-user macOS LaunchAgent (`~/Library/LaunchAgents/de.d56.shrike.watch.plist`) that runs `shrike watch` at login and relaunches it if it crashes. Logs go to `shrike.log`. Remove it with `--uninstall`.
```

- [ ] **Step 4: CHANGELOG — Unreleased / Added**

In `CHANGELOG.md`, under `## [Unreleased]` → `### Added`, append:

```markdown
- `shrike watch --install` / `--uninstall` / `--status`: register `shrike watch` as a per-user
  macOS LaunchAgent so it runs in the background and starts at login (relaunches on crash via
  `KeepAlive`). The agent runs `watch --quiet`, logs to `shrike.log`, and gets a Homebrew-aware
  `PATH` so `terminal-notifier` works.
```

- [ ] **Step 5: Commit**

```bash
git add README.md CHANGELOG.md
git commit -m "docs: document shrike watch LaunchAgent flags"
```

- [ ] **Step 6: Final verification**

```
go test ./... -race -count=1 && go vet ./...
```

Expected: all green, no vet output.

---

## Self-Review notes (resolved during planning)

- **Spec coverage:** `Plist` + `Manager` (Install/Uninstall/Loaded) + targets + idempotent reload (T1); `--install`/`--uninstall`/`--status` flags + mutual-exclusion + management branch reusing `os.Executable`/`history.LogPath` (T2); docs + safe smoke (T3). All spec sections mapped.
- **Safety:** no automated step runs a real `--install`; agent tests use the injected `run`; the only real launchctl call is read-only `--status`. Stated at the top and in T3.
- **launchctl args:** `bootstrap gui/<uid> <plist>`, `bootout gui/<uid>/<label>`, `print gui/<uid>/<label>` — asserted exactly in `TestInstall_WritesPlistAndBootstraps`.
- **Decoupling:** `internal/agent` does not import `internal/history`; the cmd resolves `execPath`/`logPath` and passes them to `NewManager`.
- **Type consistency:** `agent.Label`, `agent.Plist(label, execPath, logPath) string`, `agent.NewManager(execPath, logPath) (*Manager, error)`, `Manager.{Install,Uninstall,Loaded,PlistPath,LogPath}`, `handleAgentFlags(cmd) (bool, error)`, and the three `watch*` flag vars are used consistently across T1–T2. The default `run` discards launchctl output (nil stdout/stderr), so `--status` stays quiet.
