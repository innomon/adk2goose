package config

import (
	"os"
	"time"
)

type Config struct {
	GooseBaseURL   string
	GooseSecret    string
	ListenAddr     string
	WorkingDir     string
	RequestTimeout time.Duration
}

func Load() (*Config, error) {
	cfg := &Config{
		GooseBaseURL:   envOrDefault("GOOSE_BASE_URL", "http://127.0.0.1:3000"),
		GooseSecret:    os.Getenv("GOOSE_SECRET_KEY"),
		ListenAddr:     envOrDefault("LISTEN_ADDR", ":8080"),
		WorkingDir:     envOrDefault("WORKING_DIR", "."),
		RequestTimeout: 5 * time.Minute,
	}

	if v := os.Getenv("REQUEST_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, err
		}
		cfg.RequestTimeout = d
	}

	return cfg, nil
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
