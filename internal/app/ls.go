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

func runLs(_ *cobra.Command, _ []string) error {
	entries, err := ports.ListListening()
	if err != nil {
		return err
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "PROTO\tLOCAL ADDR\tPORT")
	for _, e := range entries {
		fmt.Fprintf(w, "%s\t%s\t%d\n", e.Proto, e.LocalAddr, e.Port)
	}

	return w.Flush()
}
