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
	if err := os.MkdirAll(filepath.Dir(m.PlistPath), 0o750); err != nil {
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

// Loaded reports whether the agent is currently bootstrapped. Any launchctl
// failure (including "no such service") is treated as not loaded — this is a
// read-only status probe, so an erroring/absent agent is effectively not running.
func (m *Manager) Loaded() bool {
	return m.run("print", m.serviceTarget()) == nil
}
