package app

import (
	"github.com/eslusarenko/port-client/internal/version"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:     "port",
	Short:   "Inspect and manage network ports",
	Version: version.Version,
}

// Execute runs the root command.
func Execute() {
	_ = rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(lsCmd)
}
