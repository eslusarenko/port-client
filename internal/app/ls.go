package app

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/eslusarenko/port-client/internal/ports"
	"github.com/spf13/cobra"
)

var lsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List all TCP listening ports",
	RunE:  runLs,
}

func init() {
	lsCmd.Flags().BoolP("verbose", "v", false, "Show full command line and arguments")
}

func runLs(cmd *cobra.Command, _ []string) error {
	verbose, _ := cmd.Flags().GetBool("verbose")

	entries, err := ports.ListListening()
	if err != nil {
		return err
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)

	if verbose {
		fmt.Fprintln(w, "PROTO\tLOCAL ADDR\tPORT\tPID\tPROCESS\tCOMMAND")
		for _, e := range entries {
			fmt.Fprintf(w, "%s\t%s\t%d\t%d\t%s\t%s\n", e.Proto, e.LocalAddr, e.Port, e.PID, e.Process, e.CmdLine)
		}
	} else {
		fmt.Fprintln(w, "PROTO\tLOCAL ADDR\tPORT\tPID\tPROCESS")
		for _, e := range entries {
			fmt.Fprintf(w, "%s\t%s\t%d\t%d\t%s\n", e.Proto, e.LocalAddr, e.Port, e.PID, e.Process)
		}
	}

	return w.Flush()
}
