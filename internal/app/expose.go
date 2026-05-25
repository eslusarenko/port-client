package app

import (
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strconv"

	"github.com/eslusarenko/port-client/internal/config"
	"github.com/eslusarenko/port-client/internal/tunnel"
	"github.com/spf13/cobra"
)

var exposeCmd = &cobra.Command{
	Use:   "expose <port>",
	Short: "Expose a local service via a public tunnel",
	Long: "Creates a tunnel to the server, generating a public URL that forwards traffic to the local port.\n\n" +
		"Requests are printed by default. Use --silence (-s) to suppress request output.",
	Example: `  port expose 8080
  port expose --silence 8891
  port expose --all-request-headers 8891
  port expose --request-headers host,user-agent 8891
  port expose --set-host localhost:8891 8891
  port expose --server wss://server.example.com 8891`,
	Args: cobra.ExactArgs(1),
	RunE: runExpose,
}

func init() {
	exposeCmd.Flags().String("server", "", "Server WebSocket URL (overrides PORT_SERVER env var)")
	exposeCmd.Flags().BoolP("silence", "s", false, "Suppress request output (requests are printed by default)")
	exposeCmd.Flags().Bool("all-request-headers", false, "Print all request headers")
	exposeCmd.Flags().String("request-headers", "", "Print only these request headers (comma-separated, e.g. host,user-agent)")
	exposeCmd.Flags().String("set-host", "", "Override the Host header sent to the local app")
	exposeCmd.Flags().String("domain", "", "Request a specific subdomain (e.g. --domain myfeature for myfeature.tnls.lt)")
}

func runExpose(cmd *cobra.Command, args []string) error {
	port, err := strconv.Atoi(args[0])
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("invalid port %q: must be an integer between 1 and 65535", args[0])
	}

	targetURL := &url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("localhost:%d", port),
	}

	cfg := config.Load()
	if serverFlag, _ := cmd.Flags().GetString("server"); serverFlag != "" {
		cfg.ServerAddr = serverFlag
	}

	silence, _ := cmd.Flags().GetBool("silence")
	logHeaders, _ := cmd.Flags().GetBool("all-request-headers")
	headerFilter, _ := cmd.Flags().GetString("request-headers")
	setHost, _ := cmd.Flags().GetString("set-host")
	domain, _ := cmd.Flags().GetString("domain")

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	print := tunnel.PrintConfig{
		Requests:     !silence,
		Headers:      logHeaders,
		HeaderFilter: headerFilter,
	}

	client := tunnel.New(cfg.ServerAddr, targetURL, logger, cfg.MaxBodySize, print, setHost, domain, cfg.APIKey)

	publicURL, err := client.Connect(cmd.Context())
	if err != nil {
		return fmt.Errorf("connect to server: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Tunnel ready: %s -> %s\n", publicURL, targetURL)
	fmt.Println(publicURL)

	client.Wait()
	return nil
}
