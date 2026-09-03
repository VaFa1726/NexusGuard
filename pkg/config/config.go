package config

import (
	"fmt"
	"os"
)

type Config struct {
	TelegramToken string
	DatabaseURL   string
	ProxyURL      string
}

func LoadConfig() (*Config, error) {
	token := os.Getenv("TELEGRAM_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("TELEGRAM_TOKEN is not set")
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is not set")
	}

	// PROXY_URL is optional
	proxyURL := os.Getenv("PROXY_URL")

	return &Config{
		TelegramToken: token,
		DatabaseURL:   dbURL,
		ProxyURL:      proxyURL,
	}, nil
}
