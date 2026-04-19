package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	actions_ "github.com/d56de/shrike/internal/actions"
	cfg "github.com/d56de/shrike/internal/config"
	"github.com/d56de/shrike/internal/core"
	"github.com/d56de/shrike/internal/detectors"
	"github.com/d56de/shrike/internal/history"
	"github.com/d56de/shrike/internal/sysinfo"
	"github.com/d56de/shrike/internal/tui/doctor"
	"github.com/d56de/shrike/internal/tui/style"
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

		// If scanning takes longer than 300ms, show an animated braille spinner
		// in #8A88C2 on stderr. Silent for fast scans. Cleared before the TUI
		// opens its alt-screen.
		done := make(chan struct{})
		go spinScanning(done)

		start := time.Now()
		findings, err := engine.Run(context.Background())
		close(done)
		if err != nil {
			return fmt.Errorf("engine run: %w", err)
		}
		elapsed := time.Since(start)

		if c.History.Enabled {
			_ = history.RotateIfNeeded(c.History.MaxSizeMB, c.History.MaxRotations)
			if w, werr := history.NewWriter(); werr == nil {
				_ = w.AppendRun(history.RunMeta{
					TS:         start,
					Mode:       "doctor",
					DurationMS: elapsed.Milliseconds(),
				}, findings)
				_ = w.Close()
			}
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

		// Interactive TUI path.
		acts := []core.Action{
			actions_.NewInfo(),
			actions_.NewSample(),
			actions_.NewKill(),
			actions_.NewKillImmediate(),
			actions_.NewRenice(),
		}
		theme := style.FromConfig(
			c.UI.SeverityHighColor,
			c.UI.SeverityMediumColor,
			c.UI.SeverityLowColor,
		)
		model := doctor.NewModelWithTheme(findings, acts, theme)
		model.Version = version
		model.RunDuration = elapsed
		model.ProcsScanned = 0 // v0.2: plumb from engine snapshot
		model.Engine = engine
		model.HistoryEnabled = c.History.Enabled
		model.HistoryConfig = history.WriterConfig{
			Enabled:      c.History.Enabled,
			MaxSizeMB:    c.History.MaxSizeMB,
			MaxRotations: c.History.MaxRotations,
		}
		prog := tea.NewProgram(model, tea.WithAltScreen())
		if _, err := prog.Run(); err != nil {
			return fmt.Errorf("tui: %w", err)
		}
		return nil
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
func buildEngine(c cfg.Config, only []string) *core.Engine {
	all := []core.Detector{detectors.NewRunaway(), detectors.NewZombie(), detectors.NewHerd()}
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
		"zombie": {
			"min_age": time.Duration(c.Zombie.MinAge),
		},
		"herd": {
			"min_size":            c.Herd.MinSize,
			"total_cpu_threshold": c.Herd.TotalCPUThreshold,
		},
	}

	return &core.Engine{
		Snapshotter: sysinfo.New(),
		Detectors:   selected,
		Configs:     configs,
	}
}

// spinScanning prints an animated braille spinner + "shrike: scanning
// processes…" on stderr until done is closed. First frame appears after 300ms
// to avoid flashing for sub-second scans. Uses the Frame style colour (#8A88C2).
func spinScanning(done <-chan struct{}) {
	select {
	case <-time.After(300 * time.Millisecond):
	case <-done:
		return
	}

	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	spinStyle := style.DefaultTheme().Frame

	tick := time.NewTicker(80 * time.Millisecond)
	defer tick.Stop()

	i := 0
	for {
		select {
		case <-done:
			// Clear the line before yielding to Bubble Tea.
			_, _ = fmt.Fprint(os.Stderr, "\r\x1b[2K")
			return
		case <-tick.C:
			_, _ = fmt.Fprintf(os.Stderr, "\r%s shrike: scanning processes…",
				spinStyle.Render(frames[i%len(frames)]))
			i++
		}
	}
}

func init() {
	doctorCmd.Flags().BoolVar(&doctorJSON, "json", false, "emit JSON findings to stdout, no TUI")
	doctorCmd.Flags().StringSliceVar(&doctorOnly, "only", nil, "comma-separated detector names to run (default: all)")
}
