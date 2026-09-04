package health

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Health status information
type Status struct {
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
	Database  string    `json:"database"`
	Uptime    string    `json:"uptime"`
}

var startTime = time.Now()

// Handler provides HTTP health check endpoint
type Handler struct {
	pool *pgxpool.Pool
}

// NewHandler creates a new health check handler
func NewHandler(pool *pgxpool.Pool) *Handler {
	return &Handler{pool: pool}
}

// Start begins the health check HTTP server on the specified port
func (h *Handler) Start(port string) {
	http.HandleFunc("/health", h.healthCheck)
	http.HandleFunc("/ready", h.readinessCheck)
	http.HandleFunc("/metrics", h.metrics)

	slog.Info("Starting health check server", "port", port)
	go func() {
		if err := http.ListenAndServe(":"+port, nil); err != nil {
			slog.Error("Health check server failed", "error", err)
		}
	}()
}

// healthCheck returns basic health status
func (h *Handler) healthCheck(w http.ResponseWriter, r *http.Request) {
	status := Status{
		Status:    "healthy",
		Timestamp: time.Now(),
		Database:  "unknown",
		Uptime:    time.Since(startTime).String(),
	}

	// Quick DB ping
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := h.pool.Ping(ctx); err != nil {
		status.Database = "unhealthy"
		status.Status = "degraded"
		w.WriteHeader(http.StatusServiceUnavailable)
	} else {
		status.Database = "healthy"
		w.WriteHeader(http.StatusOK)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// readinessCheck returns whether the service is ready to accept traffic
func (h *Handler) readinessCheck(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := h.pool.Ping(ctx); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("not ready"))
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ready"))
}

// metrics returns basic metrics (can be extended for Prometheus)
func (h *Handler) metrics(w http.ResponseWriter, r *http.Request) {
	stats := h.pool.Stat()

	metrics := map[string]interface{}{
		"connections_acquired":     stats.AcquiredConns(),
		"connections_idle":         stats.IdleConns(),
		"connections_total":        stats.TotalConns(),
		"acquire_count":            stats.AcquireCount(),
		"max_conns":                stats.MaxConns(),
		"uptime_seconds":           time.Since(startTime).Seconds(),
		"new_connections_count":    stats.NewConnsCount(),
		"empty_acquire_count":      stats.EmptyAcquireCount(),
		"canceled_acquire_count":   stats.CanceledAcquireCount(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(metrics)
}
