package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/BurntSushi/toml"
	cfg "github.com/d56de/shrike/internal/config"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Show the effective config and its path",
	RunE: func(cmd *cobra.Command, _ []string) error {
		path, err := cfg.Path()
		if err != nil {
			return fmt.Errorf("resolve config path: %w", err)
		}
		c, err := cfg.Load()
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "# Config path: %s\n", path)
		return toml.NewEncoder(cmd.OutOrStdout()).Encode(c)
	},
}

var configEditCmd = &cobra.Command{
	Use:   "edit",
	Short: "Open config.toml in $EDITOR",
	RunE: func(_ *cobra.Command, _ []string) error {
		path, err := cfg.Path()
		if err != nil {
			return fmt.Errorf("resolve config path: %w", err)
		}
		// Ensure the file exists so $EDITOR has something to open.
		if _, err := os.Stat(path); os.IsNotExist(err) {
			if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
				return fmt.Errorf("mkdir config dir: %w", err)
			}
			if err := writeDefaultConfig(path); err != nil {
				return fmt.Errorf("write default config: %w", err)
			}
		}
		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = "vi"
		}
		c := exec.Command(editor, path) //nolint:gosec // editor comes from $EDITOR which is user-controlled by design
		c.Stdin = os.Stdin
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		return c.Run()
	},
}

func writeDefaultConfig(path string) error {
	f, err := os.Create(path) //nolint:gosec // path derived from XDG_CONFIG_HOME or user home
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return toml.NewEncoder(f).Encode(cfg.DefaultConfig())
}

func init() {
	configCmd.AddCommand(configEditCmd)
}
