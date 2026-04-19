package history

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRotate_RenamesWhenOversize(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tmp)

	dir := filepath.Join(tmp, "shrike")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "history.jsonl")

	if err := os.WriteFile(path, []byte(strings.Repeat("x", 11*1024*1024)), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := RotateIfNeeded(10, 3); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("expected original file to be renamed away")
	}
	if _, err := os.Stat(path + ".1"); err != nil {
		t.Errorf("expected history.jsonl.1 to exist: %v", err)
	}
}

func TestRotate_BelowThreshold_NoOp(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tmp)

	dir := filepath.Join(tmp, "shrike")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "history.jsonl")
	if err := os.WriteFile(path, []byte("tiny"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := RotateIfNeeded(10, 3); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Error("file should still exist")
	}
}
