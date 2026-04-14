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
  port expose --server ws://tunnel.example.com http://localhost:8891`,
	Args: cobra.ExactArgs(1),
	RunE: runExpose,
}

func init() {
	exposeCmd.Flags().String("server", "", "Server WebSocket URL (overrides PORT_SERVER env var)")
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

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	client := tunnel.New(cfg.ServerAddr, targetURL, logger, cfg.MaxBodySize)

	publicURL, err := client.Connect(cmd.Context())
	if err != nil {
		return fmt.Errorf("connect to server: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Tunnel ready: %s -> %s\n", publicURL, args[0])
	fmt.Println(publicURL)

	client.Wait()
	return nil
}
