package logger

import (
	"log/slog"
	"os"
)

func InitLogger() {
	// JSON handler is great for production and parsing logs with journalctl/fluentd
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	logger := slog.New(handler)
	slog.SetDefault(logger)
}
