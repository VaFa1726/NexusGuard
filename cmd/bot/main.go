package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nexusguard/bot/pkg/config"
	"github.com/nexusguard/bot/pkg/database"
	"github.com/nexusguard/bot/pkg/logger"
	
	"golang.org/x/net/proxy"
	tele "gopkg.in/telebot.v3"
)

func main() {
	// Initialize Logger
	logger.InitLogger()
	slog.Info("Starting NexusGuard Bot...")

	// Load Configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		slog.Error("Failed to load config", "error", err)
		os.Exit(1)
	}

	// Initialize Database
	db, err := database.Connect(cfg.DatabaseURL)
	if err != nil {
		slog.Error("Failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer database.Close(db)
	slog.Info("Connected to PostgreSQL successfully")

	// Set up HTTP Client with Proxy (for Iran servers)
	var httpClient *http.Client
	if cfg.ProxyURL != "" {
		slog.Info("Using proxy", "url", cfg.ProxyURL)
		// Basic setup for SOCKS5/HTTP Proxy (Assuming http or socks5)
		// For a robust implementation, use golang.org/x/net/proxy
		os.Setenv("HTTP_PROXY", cfg.ProxyURL)
		os.Setenv("HTTPS_PROXY", cfg.ProxyURL)
		httpClient = &http.Client{}
	} else {
		httpClient = &http.Client{}
	}

	// Initialize Telegram Bot
	pref := tele.Settings{
		Token:  cfg.TelegramToken,
		Poller: &tele.LongPoller{Timeout: 10 * time.Second},
		Client: httpClient,
	}

	bot, err := tele.NewBot(pref)
	if err != nil {
		slog.Error("Failed to initialize telegram bot", "error", err)
		os.Exit(1)
	}

	// Basic Ping-Pong Handler
	bot.Handle("/ping", func(c tele.Context) error {
		return c.Send("Pong! 🏓 NexusGuard is active.")
	})

	// Start Command Handler
	bot.Handle("/start", func(c tele.Context) error {
		return c.Send("سلام! به NexusGuard خوش آمدید. 🛡️\nمن اینجا هستم تا به شما کمک کنم.")
	})

	// Graceful Shutdown Setup
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		slog.Info("Bot is now running. Press Ctrl+C to stop.")
		bot.Start()
	}()

	<-ctx.Done()
	slog.Info("Shutting down gracefully...")
	bot.Stop()
	slog.Info("NexusGuard stopped.")
}
