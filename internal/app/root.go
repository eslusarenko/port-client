package app

import (
	"os"

	"github.com/eslusarenko/port-client/internal/version"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:     "port",
	Short:   "Inspect and manage network ports",
	Version: version.Version,
}

// Execute runs the root command and exits with code 1 on failure.
// Cobra already prints the error to stderr before returning it.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(lsCmd)
}
