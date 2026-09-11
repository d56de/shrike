package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGPUConfigLoadsAndMergesIgnores(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	p, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(p, []byte("[gpu]\nthreshold = 95\nmin_duration = '45s'\nignore = ['Renderer']\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err = AppendIgnore("gpu", "Game"); err != nil {
		t.Fatal(err)
	}
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !c.GPU.Enabled || c.GPU.Threshold != 95 || time.Duration(c.GPU.MinDuration) != 45*time.Second || len(c.GPU.Ignore) != 2 {
		t.Fatalf("config = %#v", c.GPU)
	}
}
