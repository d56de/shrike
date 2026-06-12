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
