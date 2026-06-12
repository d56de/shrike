package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var version = "dev" // set by goreleaser via -ldflags

var rootCmd = &cobra.Command{
	Use:   "shrike",
	Short: "Hunt runaway, zombie, and herd processes on your Mac",
	Long: `shrike surfaces suspicious processes using opinionated heuristics
(runaway CPU, zombies, helper-process herds) and lets you act on them
with single keystrokes.

Running 'shrike' with no subcommand is equivalent to 'shrike doctor'.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return doctorCmd.RunE(cmd, args)
	},
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(doctorCmd)
	rootCmd.AddCommand(logCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(statsCmd)
	rootCmd.AddCommand(watchCmd)
}
