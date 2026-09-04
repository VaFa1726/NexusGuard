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

	"github.com/nexusguard/bot/internal/delivery/telegram"
	"github.com/nexusguard/bot/internal/repository/postgres"
	"github.com/nexusguard/bot/internal/usecase"
	"github.com/nexusguard/bot/pkg/config"
	"github.com/nexusguard/bot/pkg/database"
	"github.com/nexusguard/bot/pkg/logger"

	goProxy "golang.org/x/net/proxy"
	tele "gopkg.in/telebot.v3"
)

func main() {
	// ── Logger ───────────────────────────────────────────────────────────────
	logger.InitLogger()
	slog.Info("Starting NexusGuard Bot...", "version", "1.0.0")

	// ── Config ───────────────────────────────────────────────────────────────
	cfg, err := config.LoadConfig()
	if err != nil {
		slog.Error("Failed to load config", "error", err)
		os.Exit(1)
	}

	// ── Database ─────────────────────────────────────────────────────────────
	pool, err := database.Connect(cfg.DatabaseURL)
	if err != nil {
		slog.Error("Failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer database.Close(pool)

	// Run migrations
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	if err := postgres.RunMigrations(ctx, pool); err != nil {
		cancel()
		slog.Error("Database migration failed", "error", err)
		os.Exit(1)
	}
	cancel()

	// ── HTTP Client with Proxy ────────────────────────────────────────────────
	httpClient, err := buildHTTPClient(cfg.ProxyURL)
	if err != nil {
		slog.Error("Failed to configure HTTP client", "error", err)
		os.Exit(1)
	}

	// ── Telegram Bot ─────────────────────────────────────────────────────────
	pref := tele.Settings{
		Token: cfg.TelegramToken,
		Poller: &tele.LongPoller{
			Timeout: 10 * time.Second,
			// Must explicitly allow my_chat_member to detect when bot is added to groups
			AllowedUpdates: []string{
				"message",
				"callback_query",
				"my_chat_member",
				"chat_member",
			},
		},
		Client: httpClient,
	}

	bot, err := tele.NewBot(pref)
	if err != nil {
		slog.Error("Failed to initialize telegram bot", "error", err)
		os.Exit(1)
	}
	slog.Info("Telegram bot connected", "username", bot.Me.Username, "id", bot.Me.ID)

	// ── Dependency Injection ─────────────────────────────────────────────────
	groupRepo := postgres.NewGroupRepository(pool)
	userRepo := postgres.NewUserRepository(pool)
	adminRepo := postgres.NewAdminRepository(pool)

	groupSvc := usecase.NewGroupService(groupRepo, userRepo)
	adminSvc := usecase.NewAdminService(adminRepo, groupRepo)

	handler := telegram.NewHandler(groupSvc, adminSvc, adminRepo)

	// ── Register Handlers ─────────────────────────────────────────────────────
	handler.RegisterAll(bot)

	// ── Graceful Shutdown ─────────────────────────────────────────────────────
	shutdownCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		slog.Info("NexusGuard is now running ✅", "bot", "@"+bot.Me.Username)
		bot.Start()
	}()

	<-shutdownCtx.Done()
	slog.Info("Shutdown signal received, stopping bot...")
	bot.Stop()
	slog.Info("NexusGuard stopped gracefully. Goodbye! 👋")
}

// buildHTTPClient creates an *http.Client with optional SOCKS5 or HTTP proxy.
func buildHTTPClient(proxyURL string) (*http.Client, error) {
	if proxyURL == "" {
		return &http.Client{Timeout: 30 * time.Second}, nil
	}

	slog.Info("Configuring proxy", "url", proxyURL)

	if strings.HasPrefix(proxyURL, "socks5://") {
		addr := strings.TrimPrefix(proxyURL, "socks5://")
		dialer, err := goProxy.SOCKS5("tcp", addr, nil, goProxy.Direct)
		if err != nil {
			return nil, err
		}
		transport := &http.Transport{
			DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
				return dialer.Dial(network, address)
			},
		}
		return &http.Client{Transport: transport, Timeout: 30 * time.Second}, nil
	}

	// HTTP / HTTPS proxy
	parsed, err := url.Parse(proxyURL)
	if err != nil {
		return nil, err
	}
	transport := &http.Transport{Proxy: http.ProxyURL(parsed)}
	return &http.Client{Transport: transport, Timeout: 30 * time.Second}, nil
}
