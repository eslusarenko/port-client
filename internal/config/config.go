package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	ServerAddr  string
	LogLevel    string
	MaxBodySize int64
	APIKey      string
}

func Load() *Config {
	apiKey := os.Getenv("PORT_API_KEY")
	if apiKey == "" {
		apiKey = apiKeyFromPortConf()
	}

	return &Config{
		ServerAddr:  envOr("PORT_SERVER", "wss://pm.tnls.lt"),
		LogLevel:    envOr("PORT_LOG_LEVEL", "info"),
		MaxBodySize: 10 << 20, // 10 MB
		APIKey:      apiKey,
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func apiKeyFromPortConf() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}

	f, err := os.Open(filepath.Join(home, ".port.conf"))
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}

		if strings.TrimSpace(key) == "PORT_API_KEY" {
			return strings.TrimSpace(value)
		}
	}

	// best-effort parsing: ignore scanner errors
	return ""
}
