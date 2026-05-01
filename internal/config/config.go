// Package config loads and represents the shrike TOML config.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
)

// Config is the root configuration object.
type Config struct {
	General GeneralConfig `toml:"general"`
	Runaway RunawayConfig `toml:"runaway"`
	Zombie  ZombieConfig  `toml:"zombie"`
	Herd    HerdConfig    `toml:"herd"`
	History HistoryConfig `toml:"history"`
	UI      UIConfig      `toml:"ui"`
}

// GeneralConfig holds top-level settings.
type GeneralConfig struct {
	DefaultMode string `toml:"default_mode"`
}

// RunawayConfig configures the runaway detector.
type RunawayConfig struct {
	CPUThreshold float64  `toml:"cpu_threshold"`
	MinAge       Duration `toml:"min_age"`
	Ignore       []string `toml:"ignore"`
}

// ZombieConfig configures the zombie detector.
type ZombieConfig struct {
	MinAge Duration `toml:"min_age"`
	Ignore []string `toml:"ignore"`
}

// HerdConfig configures the herd detector.
type HerdConfig struct {
	MinSize           int      `toml:"min_size"`
	TotalCPUThreshold float64  `toml:"total_cpu_threshold"`
	KnownBadActors    []string `toml:"known_bad_actors"`
	Ignore            []string `toml:"ignore"`
}

// HistoryConfig controls the history JSONL behaviour.
type HistoryConfig struct {
	Enabled      bool `toml:"enabled"`
	MaxSizeMB    int  `toml:"max_size_mb"`
	MaxRotations int  `toml:"max_rotations"`
}

// UIConfig holds colours for the TUI.
type UIConfig struct {
	SeverityHighColor   string `toml:"severity_high_color"`
	SeverityMediumColor string `toml:"severity_medium_color"`
	SeverityLowColor    string `toml:"severity_low_color"`
}

// Duration wraps time.Duration so TOML can unmarshal human-readable strings
// like "1h", "5m".
type Duration time.Duration

// UnmarshalText parses a duration string into the wrapped time.Duration.
func (d *Duration) UnmarshalText(text []byte) error {
	parsed, err := time.ParseDuration(string(text))
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", string(text), err)
	}
	*d = Duration(parsed)
	return nil
}

// MarshalText serialises the duration back to its canonical Go representation.
func (d Duration) MarshalText() ([]byte, error) {
	return []byte(time.Duration(d).String()), nil
}

// Path returns the resolved absolute path to the config file.
//
// Resolution order: $XDG_CONFIG_HOME/shrike/config.toml → ~/.config/shrike/config.toml.
func Path() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "shrike", "config.toml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "shrike", "config.toml"), nil
}

// Load reads the config file and applies defaults for missing sections/fields.
// Returns the default config if the file does not exist.
func Load() (Config, error) {
	path, err := Path()
	if err != nil {
		return Config{}, err
	}
	cfg := DefaultConfig()
	data, err := os.ReadFile(path) //nolint:gosec // path is derived from XDG_CONFIG_HOME or user home, not user input
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("read config %s: %w", path, err)
	}
	if _, err := toml.Decode(string(data), &cfg); err != nil {
		return Config{}, fmt.Errorf("decode config %s: %w", path, err)
	}
	return cfg, nil
}
