package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/d56de/shrike/internal/agent"
	cfg "github.com/d56de/shrike/internal/config"
	"github.com/d56de/shrike/internal/core"
	"github.com/d56de/shrike/internal/detectors"
	"github.com/d56de/shrike/internal/history"
	"github.com/d56de/shrike/internal/notify"
	"github.com/d56de/shrike/internal/watch"
	"github.com/spf13/cobra"
)

var (
	watchInterval  time.Duration
	watchLevel     string
	watchQuiet     bool
	watchInstall   bool
	watchUninstall bool
	watchStatus    bool
)

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Continuously scan and notify on new suspicious processes",
	Long: `Run shrike in the foreground, re-scanning every interval and sending a
macOS notification when a new (or escalating) finding appears at or above the
notify level. Each scan is appended to the history file. Press Ctrl-C to stop.

Examples:
  shrike watch                       # scan every 60s, notify on high+critical
  shrike watch --interval 30s        # faster cadence
  shrike watch --notify-level critical
  shrike watch --quiet               # only print when notifying`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		if handled, err := handleAgentFlags(cmd); handled || err != nil {
			return err
		}

		c, err := cfg.Load()
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}

		interval := time.Duration(c.Watch.Interval)
		if cmd.Flags().Changed("interval") {
			interval = watchInterval
		}
		if interval <= 0 {
			return fmt.Errorf("--interval must be > 0, got %v", interval)
		}

		levelStr := c.Watch.NotifyLevel
		if cmd.Flags().Changed("notify-level") {
			levelStr = watchLevel
		}
		level, err := parseLevel(levelStr)
		if err != nil {
			return err
		}

		engine := buildEngine(c, nil)
		w := watch.NewWatcher(level)
		notifier := notify.NewSystem()
		out := cmd.OutOrStdout()

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		scan := func() {
			findings, err := engine.Run(ctx)
			if err != nil {
				_, _ = fmt.Fprintf(os.Stderr, "shrike watch: scan error: %v\n", err)
				return
			}
			writeWatchHistory(c, findings)
			if !watchQuiet {
				_, _ = fmt.Fprintln(out, watchLine(findings))
			}
			for _, note := range w.Decide(findings) {
				if err := notifier.Notify(note); err != nil {
					_, _ = fmt.Fprintf(os.Stderr, "shrike watch: notify error: %v\n", err)
				}
			}
		}

		scan() // immediate first scan
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				_, _ = fmt.Fprintln(out, "shrike watch stopped")
				return nil
			case <-ticker.C:
				scan()
			}
		}
	},
}

// parseLevel maps a config/flag string to a core.Severity.
func parseLevel(s string) (core.Severity, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "low":
		return core.SeverityLow, nil
	case "medium":
		return core.SeverityMedium, nil
	case "high":
		return core.SeverityHigh, nil
	case "critical":
		return core.SeverityCritical, nil
	default:
		return 0, fmt.Errorf("invalid notify level %q (want low|medium|high|critical)", s)
	}
}

// handleAgentFlags processes the --install/--uninstall/--status management
// flags. It returns handled=true when one was set (so the foreground loop is
// skipped), along with the action's error. With no flag set it returns
// (false, nil) so the caller runs the watch loop.
func handleAgentFlags(cmd *cobra.Command) (bool, error) {
	n := 0
	for _, b := range []bool{watchInstall, watchUninstall, watchStatus} {
		if b {
			n++
		}
	}
	if n == 0 {
		return false, nil
	}
	if n > 1 {
		return true, fmt.Errorf("--install, --uninstall, and --status are mutually exclusive")
	}

	exe, err := os.Executable()
	if err != nil {
		return true, fmt.Errorf("resolve shrike binary path: %w", err)
	}
	logPath, err := history.LogPath()
	if err != nil {
		return true, err
	}
	mgr, err := agent.NewManager(exe, logPath)
	if err != nil {
		return true, err
	}
	out := cmd.OutOrStdout()

	switch {
	case watchInstall:
		if err := mgr.Install(); err != nil {
			return true, err
		}
		_, _ = fmt.Fprintf(out, "✓ shrike watch installed as a LaunchAgent — starts at login.\n  Logs: %s\n", mgr.LogPath)
	case watchUninstall:
		if err := mgr.Uninstall(); err != nil {
			return true, err
		}
		_, _ = fmt.Fprintln(out, "✓ shrike watch LaunchAgent removed")
	case watchStatus:
		if mgr.Loaded() {
			_, _ = fmt.Fprintf(out, "running (LaunchAgent loaded)\n  plist: %s\n", mgr.PlistPath)
		} else {
			_, _ = fmt.Fprintln(out, "not installed\n  (run `shrike watch --install` to run in the background)")
		}
	}
	return true, nil
}

// watchLine formats the one-line per-scan status output.
func watchLine(findings []core.Finding) string {
	ts := time.Now().Format("15:04:05")
	if len(findings) == 0 {
		return ts + "  ✓ clean"
	}
	word := "findings"
	if len(findings) == 1 {
		word = "finding"
	}
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "%s  %d %s", ts, len(findings), word)
	for _, f := range findings {
		_, _ = fmt.Fprintf(&b, "  %s %s", detectors.Emoji(f.Detector), f.Process.Command)
	}
	return b.String()
}

// writeWatchHistory appends one watch run to the history file, best-effort,
// reusing the same rotate+append path as the doctor TUI rescan.
func writeWatchHistory(c cfg.Config, findings []core.Finding) {
	if !c.History.Enabled {
		return
	}
	_ = history.RotateIfNeeded(c.History.MaxSizeMB, c.History.MaxRotations)
	w, err := history.NewWriter()
	if err != nil {
		return
	}
	defer func() { _ = w.Close() }()
	_ = w.AppendRun(history.RunMeta{TS: time.Now(), Mode: "watch"}, findings)
}

func init() {
	watchCmd.Flags().DurationVar(&watchInterval, "interval", 60*time.Second, "scan interval (overrides config)")
	watchCmd.Flags().StringVar(&watchLevel, "notify-level", "high", "minimum severity to notify: low|medium|high|critical")
	watchCmd.Flags().BoolVar(&watchQuiet, "quiet", false, "suppress the per-scan status line")
	watchCmd.Flags().BoolVar(&watchInstall, "install", false, "install shrike watch as a background LaunchAgent (starts at login)")
	watchCmd.Flags().BoolVar(&watchUninstall, "uninstall", false, "remove the shrike watch LaunchAgent")
	watchCmd.Flags().BoolVar(&watchStatus, "status", false, "show whether the LaunchAgent is installed")
}
