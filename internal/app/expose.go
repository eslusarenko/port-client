package app

import (
	"fmt"
	"log/slog"
	"net/url"
	"os"

	"github.com/eslusarenko/port-client/internal/config"
	"github.com/eslusarenko/port-client/internal/tunnel"
	"github.com/spf13/cobra"
)

var exposeCmd = &cobra.Command{
	Use:   "expose <url>",
	Short: "Expose a local service via a public tunnel",
	Long:  "Creates a tunnel to the server, generating a public URL that forwards traffic to the specified local service.",
	Example: `  port expose http://localhost:8080
  port expose http://127.0.0.1:3000
  port expose --server wss://pm.tnls.lt http://localhost:8891
  port expose --requests http://localhost:8891
  port expose --headers http://localhost:8891
  port expose --header host,user-agent http://localhost:8891
  port expose --requests --header host,user-agent http://localhost:8891
  port expose --set-host localhost:8891 http://localhost:8891`,
	Args: cobra.ExactArgs(1),
	RunE: runExpose,
}

func init() {
	exposeCmd.Flags().String("server", "", "Server WebSocket URL (overrides PORT_SERVER env var)")
	exposeCmd.Flags().Bool("requests", false, "Print incoming requests (METHOD URI → STATUS)")
	exposeCmd.Flags().Bool("headers", false, "Print all request headers")
	exposeCmd.Flags().String("header", "", "Print only these request headers (comma-separated, e.g. host,user-agent)")
	exposeCmd.Flags().String("set-host", "", "Override the Host header sent to the local app")
	exposeCmd.Flags().String("domain", "", "Request a specific subdomain (e.g. --domain api for api.tnls.lt)")
}

func runExpose(cmd *cobra.Command, args []string) error {
	targetURL, err := url.Parse(args[0])
	if err != nil {
		return fmt.Errorf("invalid target URL: %w", err)
	}
	if targetURL.Scheme != "http" && targetURL.Scheme != "https" {
		return fmt.Errorf("target URL scheme must be http or https, got %q", targetURL.Scheme)
	}

	cfg := config.Load()
	if serverFlag, _ := cmd.Flags().GetString("server"); serverFlag != "" {
		cfg.ServerAddr = serverFlag
	}

	logRequests, _ := cmd.Flags().GetBool("requests")
	logHeaders, _ := cmd.Flags().GetBool("headers")
	headerFilter, _ := cmd.Flags().GetString("header")
	setHost, _ := cmd.Flags().GetString("set-host")
	domain, _ := cmd.Flags().GetString("domain")

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	print := tunnel.PrintConfig{
		Requests:     logRequests,
		Headers:      logHeaders,
		HeaderFilter: headerFilter,
	}

	client := tunnel.New(cfg.ServerAddr, targetURL, logger, cfg.MaxBodySize, print, setHost, domain)

	publicURL, err := client.Connect(cmd.Context())
	if err != nil {
		return fmt.Errorf("connect to server: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Tunnel ready: %s -> %s\n", publicURL, args[0])
	fmt.Println(publicURL)

	client.Wait()
	return nil
}
