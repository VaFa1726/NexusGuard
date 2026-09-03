package main

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/nexusguard/bot/pkg/config"
	"github.com/nexusguard/bot/pkg/database"
	"github.com/nexusguard/bot/pkg/logger"

	"golang.org/x/net/proxy"
	tele "gopkg.in/telebot.v3"
)

func main() {
	// Initialize structured JSON logger
	logger.InitLogger()
	slog.Info("Starting NexusGuard Bot...")

	// Load configuration from environment
	cfg, err := config.LoadConfig()
	if err != nil {
		slog.Error("Failed to load config", "error", err)
		os.Exit(1)
	}

	// Initialize database connection pool
	db, err := database.Connect(cfg.DatabaseURL)
	if err != nil {
		slog.Error("Failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer database.Close(db)

	// Build HTTP client — with proper proxy support if configured
	httpClient, err := buildHTTPClient(cfg.ProxyURL)
	if err != nil {
		slog.Error("Failed to configure HTTP client", "error", err)
		os.Exit(1)
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

	// ─── Handlers ────────────────────────────────────────────────────────────

	bot.Handle("/start", func(c tele.Context) error {
		return c.Send("سلام! به NexusGuard خوش آمدید. 🛡️\nمن اینجا هستم تا به شما کمک کنم.")
	})

	bot.Handle("/ping", func(c tele.Context) error {
		return c.Send("Pong! 🏓 NexusGuard is active.")
	})

	// ─── Graceful Shutdown ────────────────────────────────────────────────────

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		slog.Info("Bot is now running", "username", bot.Me.Username)
		bot.Start()
	}()

	<-ctx.Done()
	slog.Info("Shutting down gracefully...")
	bot.Stop()
	slog.Info("NexusGuard stopped.")
}

// buildHTTPClient creates an *http.Client with optional SOCKS5 or HTTP proxy support.
func buildHTTPClient(proxyURL string) (*http.Client, error) {
	if proxyURL == "" {
		return &http.Client{}, nil
	}

	slog.Info("Configuring proxy", "url", proxyURL)

	if strings.HasPrefix(proxyURL, "socks5://") {
		addr := strings.TrimPrefix(proxyURL, "socks5://")
		dialer, err := proxy.SOCKS5("tcp", addr, nil, proxy.Direct)
		if err != nil {
			return nil, err
		}
		transport := &http.Transport{
			DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
				return dialer.Dial(network, address)
			},
		}
		return &http.Client{Transport: transport}, nil
	}

	// HTTP / HTTPS proxy
	parsed, err := url.Parse(proxyURL)
	if err != nil {
		return nil, err
	}
	transport := &http.Transport{
		Proxy: http.ProxyURL(parsed),
	}
	return &http.Client{Transport: transport}, nil
}
