package database

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Connect establishes a connection pool to PostgreSQL using pgx.
func Connect(dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database config: %w", err)
	}

	// Connection pool tuning - optimized for high concurrency (1000+ users)
	cfg.MaxConns = 100         // Increased from 10 for better concurrency
	cfg.MinConns = 20          // Increased from 2 to maintain ready connections
	cfg.MaxConnLifetime = 1 * time.Hour
	cfg.MaxConnIdleTime = 5 * time.Minute // Reduced from 30m to recycle faster
	cfg.HealthCheckPeriod = 1 * time.Minute
	
	// Additional pool settings for performance
	cfg.ConnConfig.ConnectTimeout = 10 * time.Second
	cfg.ConnConfig.RuntimeParams = map[string]string{
		"statement_timeout": "30000", // 30 seconds max query time
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Verify connectivity
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("database ping failed: %w", err)
	}

	slog.Info("Database connection pool established", "max_conns", cfg.MaxConns)
	return pool, nil
}

// Close gracefully closes the connection pool.
func Close(pool *pgxpool.Pool) {
	if pool != nil {
		pool.Close()
		slog.Info("Database connection pool closed")
	}
}
