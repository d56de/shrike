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
	Memleak sectionIgnore `toml:"memleak"`
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
	case "memleak":
		return &d.Memleak.Ignore
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
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
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
	cfg.Memleak.Ignore = mergeDedup(cfg.Memleak.Ignore, d.Memleak.Ignore)
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
