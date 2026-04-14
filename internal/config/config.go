package config

import "os"

type Config struct {
	ServerAddr  string
	LogLevel    string
	MaxBodySize int64
}

func Load() *Config {
	return &Config{
		ServerAddr:  envOr("PORT_SERVER", "ws://localhost:8080"),
		LogLevel:    envOr("PORT_LOG_LEVEL", "info"),
		MaxBodySize: 10 << 20, // 10 MB
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
