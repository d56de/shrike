package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Show or edit the config file",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("config: not implemented yet")
		return nil
	},
}

var configEditCmd = &cobra.Command{
	Use:   "edit",
	Short: "Open config.toml in $EDITOR",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("config edit: not implemented yet")
		return nil
	},
}

func init() {
	configCmd.AddCommand(configEditCmd)
}
