package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	cfg "github.com/d56de/shrike/internal/config"
	"github.com/d56de/shrike/internal/core"
	"github.com/d56de/shrike/internal/detectors"
	"github.com/d56de/shrike/internal/sysinfo"
	"github.com/spf13/cobra"
)

var (
	doctorJSON bool
	doctorOnly []string
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Scan for suspicious processes (interactive TUI)",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := cfg.Load()
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}

		engine := buildEngine(c, doctorOnly)

		start := time.Now()
		findings, err := engine.Run(context.Background())
		if err != nil {
			return fmt.Errorf("engine run: %w", err)
		}

		if doctorJSON {
			enc := json.NewEncoder(os.Stdout)
			for _, f := range findings {
				if err := enc.Encode(findingToJSON(start, f)); err != nil {
					return fmt.Errorf("encode finding: %w", err)
				}
			}
			if len(findings) > 0 {
				os.Exit(1)
			}
			return nil
		}

		return errors.New("interactive TUI not implemented yet; use --json")
	},
}

// findingToJSON shapes a Finding into the wire format documented in
// docs/superpowers/specs/2026-04-19-shrike-design.md § 7.
func findingToJSON(ts time.Time, f core.Finding) map[string]any {
	return map[string]any{
		"ts":       ts.UTC().Format(time.RFC3339),
		"detector": f.Detector,
		"severity": f.Severity.String(),
		"score":    f.Score,
		"reason":   f.Reason,
		"process": map[string]any{
			"pid":            f.Process.PID,
			"command":        f.Process.Command,
			"fullPath":       f.Process.FullPath,
			"user":           f.Process.User,
			"cpuPercent":     f.Process.CPUPercent,
			"memPercent":     f.Process.MemPercent,
			"rss":            f.Process.RSS,
			"elapsedSeconds": int64(f.Process.ElapsedTime.Seconds()),
			"state":          f.Process.State.String(),
		},
	}
}

// buildEngine assembles the engine with the requested detectors. If only is
// empty, all registered detectors run.
//
// v0.1 partial wiring: Zombie and Herd detectors are not yet implemented —
// only Runaway is registered. Tasks 3.2 and 3.3 will add the other two.
func buildEngine(c cfg.Config, only []string) *core.Engine {
	all := []core.Detector{detectors.NewRunaway()}
	selected := all
	if len(only) > 0 {
		wanted := map[string]bool{}
		for _, n := range only {
			wanted[n] = true
		}
		selected = nil
		for _, d := range all {
			if wanted[d.Name()] {
				selected = append(selected, d)
			}
		}
	}

	configs := map[string]core.DetectorConfig{
		"runaway": {
			"cpu_threshold": c.Runaway.CPUThreshold,
			"min_age":       time.Duration(c.Runaway.MinAge),
			"ignore":        c.Runaway.Ignore,
		},
	}

	return &core.Engine{
		Snapshotter: sysinfo.New(),
		Detectors:   selected,
		Configs:     configs,
	}
}

func init() {
	doctorCmd.Flags().BoolVar(&doctorJSON, "json", false, "emit JSON findings to stdout, no TUI")
	doctorCmd.Flags().StringSliceVar(&doctorOnly, "only", nil, "comma-separated detector names to run (default: all)")
}
