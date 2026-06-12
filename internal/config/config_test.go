package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultConfig_HasSensibleRunawayDefaults(t *testing.T) {
	c := DefaultConfig()
	if c.Runaway.CPUThreshold != 50.0 {
		t.Errorf("expected CPUThreshold=50.0, got %v", c.Runaway.CPUThreshold)
	}
	if time.Duration(c.Runaway.MinAge) != 1*time.Hour {
		t.Errorf("expected MinAge=1h, got %v", time.Duration(c.Runaway.MinAge))
	}
}

func TestDuration_UnmarshalText_Valid(t *testing.T) {
	var d Duration
	if err := d.UnmarshalText([]byte("90m")); err != nil {
		t.Fatal(err)
	}
	if time.Duration(d) != 90*time.Minute {
		t.Errorf("expected 90m, got %v", time.Duration(d))
	}
}

func TestDuration_UnmarshalText_Invalid(t *testing.T) {
	var d Duration
	if err := d.UnmarshalText([]byte("not-a-duration")); err == nil {
		t.Error("expected error, got nil")
	}
}

func TestLoad_MissingFileReturnsDefault(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Runaway.CPUThreshold != 50.0 {
		t.Error("expected defaults")
	}
}

func TestLoad_OverridesDefault(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	dir := filepath.Join(tmp, "shrike")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := `
[runaway]
cpu_threshold = 80.0
min_age = "30m"
`
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Runaway.CPUThreshold != 80.0 {
		t.Errorf("expected 80.0, got %v", cfg.Runaway.CPUThreshold)
	}
	if time.Duration(cfg.Runaway.MinAge) != 30*time.Minute {
		t.Errorf("expected 30m, got %v", time.Duration(cfg.Runaway.MinAge))
	}
	// Defaults still present for unspecified sections
	if time.Duration(cfg.Zombie.MinAge) != 5*time.Minute {
		t.Error("expected default zombie min_age")
	}
}

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
	if time.Duration(cfg.Memleak.MinAge) != 5*time.Minute {
		t.Errorf("expected default MinAge=5m, got %v", time.Duration(cfg.Memleak.MinAge))
	}
}

func TestDefaultConfig_HasWatchDefaults(t *testing.T) {
	c := DefaultConfig()
	if time.Duration(c.Watch.Interval) != 60*time.Second {
		t.Errorf("expected interval 60s, got %v", time.Duration(c.Watch.Interval))
	}
	if c.Watch.NotifyLevel != "high" {
		t.Errorf("expected notify_level high, got %q", c.Watch.NotifyLevel)
	}
}
