// Package history writes and reads the append-only JSONL history file.
package history

import (
	"os"
	"path/filepath"
	"runtime"
)

// StateDir returns the absolute path to the directory holding history.jsonl
// and shrike.log. Honours $XDG_STATE_HOME; falls back to
// ~/Library/Application Support/shrike on macOS, ~/.local/state/shrike elsewhere.
func StateDir() (string, error) {
	if xdg := os.Getenv("XDG_STATE_HOME"); xdg != "" {
		return filepath.Join(xdg, "shrike"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, "Library", "Application Support", "shrike"), nil
	}
	return filepath.Join(home, ".local", "state", "shrike"), nil
}

// Path returns the absolute path to history.jsonl.
func Path() (string, error) {
	dir, err := StateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "history.jsonl"), nil
}

// LogPath returns the absolute path to shrike.log.
func LogPath() (string, error) {
	dir, err := StateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "shrike.log"), nil
}
