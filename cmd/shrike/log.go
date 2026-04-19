package main

import (
	"fmt"
	"time"

	"github.com/d56de/shrike/internal/history"
	"github.com/spf13/cobra"
)

var (
	logSince time.Duration
	logPID   int
)

var logCmd = &cobra.Command{
	Use:   "log",
	Short: "Show the history file",
	RunE: func(cmd *cobra.Command, _ []string) error {
		records, err := history.Read(logSince, logPID)
		if err != nil {
			return fmt.Errorf("read history: %w", err)
		}
		out := cmd.OutOrStdout()
		for _, r := range records {
			switch r.Type {
			case "run":
				_, _ = fmt.Fprintf(out, "%s  %s   %v procs scanned (%vms)\n",
					r.TS.Local().Format("15:04:05"),
					r.Raw["mode"],
					r.Raw["procs_scanned"],
					r.Raw["duration_ms"],
				)
			case "finding":
				_, _ = fmt.Fprintf(out, "%s    %s %s %s  PID %v  %s  %.1f%% / %vs\n",
					r.TS.Local().Format("15:04:05"),
					detectorEmoji(r.Raw["detector"]),
					r.Raw["detector"],
					r.Raw["severity"],
					r.Raw["pid"],
					r.Raw["command"],
					r.Raw["cpu"],
					r.Raw["elapsed_s"],
				)
			}
		}
		if len(records) == 0 {
			_, _ = fmt.Fprintln(out, "no history records")
		}
		return nil
	},
}

func detectorEmoji(name any) string {
	switch name {
	case "runaway":
		return "🔥"
	case "zombie":
		return "🧟"
	case "herd":
		return "👥"
	default:
		return "•"
	}
}

func init() {
	logCmd.Flags().DurationVar(&logSince, "since", 24*time.Hour, "show entries newer than this duration")
	logCmd.Flags().IntVar(&logPID, "pid", 0, "filter by PID")
}
