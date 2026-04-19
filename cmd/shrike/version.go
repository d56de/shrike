package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print shrike version",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("shrike %s\n", version)
		return nil
	},
}
