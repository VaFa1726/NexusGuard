package config

import (
	"fmt"
	"os"
)

// Config holds all application configuration loaded from environment variables.
type Config struct {
	TelegramToken string
	DatabaseURL   string
	ProxyURL      string
}

// LoadConfig reads configuration from environment variables.
// It returns an error if any required variable is missing.
func LoadConfig() (*Config, error) {
	token := os.Getenv("TELEGRAM_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("TELEGRAM_TOKEN is not set")
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is not set")
	}

	// PROXY_URL is optional — leave empty on unrestricted networks
	proxyURL := os.Getenv("PROXY_URL")

	return &Config{
		TelegramToken: token,
		DatabaseURL:   dbURL,
		ProxyURL:      proxyURL,
	}, nil
}
