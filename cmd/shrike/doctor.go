package main

import (
	"fmt"

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
		fmt.Println("doctor: not implemented yet")
		return nil
	},
}

func init() {
	doctorCmd.Flags().BoolVar(&doctorJSON, "json", false, "emit JSON findings to stdout, no TUI")
	doctorCmd.Flags().StringSliceVar(&doctorOnly, "only", nil, "comma-separated detector names to run (default: all)")
}
