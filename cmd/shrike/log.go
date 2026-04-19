package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

var (
	logSince time.Duration
	logPID   int
)

var logCmd = &cobra.Command{
	Use:   "log",
	Short: "Show the history file",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("log: not implemented yet")
		return nil
	},
}

func init() {
	logCmd.Flags().DurationVar(&logSince, "since", 24*time.Hour, "show entries newer than this duration")
	logCmd.Flags().IntVar(&logPID, "pid", 0, "filter by PID")
}
