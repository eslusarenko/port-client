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

	var ipv4, ipv6 []ports.Entry
	for _, e := range entries {
		if e.Proto == "tcp4" {
			ipv4 = append(ipv4, e)
		} else {
			ipv6 = append(ipv6, e)
		}
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	printSection(w, "IPv4", ipv4, verbose)
	if len(ipv4) > 0 && len(ipv6) > 0 {
		fmt.Fprintln(w)
	}
	printSection(w, "IPv6", ipv6, verbose)

	return w.Flush()
}

func printSection(w *tabwriter.Writer, title string, entries []ports.Entry, verbose bool) {
	if len(entries) == 0 {
		return
	}

	fmt.Fprintf(w, "%s\n", title)

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
}
