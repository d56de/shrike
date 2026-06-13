package agent

import (
	"encoding/xml"
	"errors"
	"io"
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
	if !newTestManager("/x.plist", &recordRun{}).Loaded() { // run returns nil → loaded
		t.Error("expected loaded when launchctl print succeeds")
	}
	if newTestManager("/x.plist", &recordRun{err: errors.New("not found")}).Loaded() {
		t.Error("expected not-loaded when launchctl print fails")
	}
}

// TestPlist_WellFormedAndEscaped proves the generated plist parses as XML and
// that hostile path/label characters are escaped, so a template typo or an
// unescaped value can't produce a plist launchctl would reject.
func TestPlist_WellFormedAndEscaped(t *testing.T) {
	out := Plist(`lbl"x`, "/p/a&b<c>/shrike", "/log.log")

	dec := xml.NewDecoder(strings.NewReader(out))
	for {
		_, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("plist is not well-formed XML: %v\n%s", err, out)
		}
	}
	for _, raw := range []string{"a&b", "<c>", `"x`} {
		if strings.Contains(out, raw) {
			t.Errorf("found unescaped %q in plist:\n%s", raw, out)
		}
	}
}
