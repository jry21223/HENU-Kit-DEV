package worker

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	redislib "github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"final-review-platform/services/worker/pkg/config"
)

type probeHandler struct {
	cfg   config.Config
	db    *gorm.DB
	cache *redislib.Client
}

func NewProbeHandler(cfg config.Config, db *gorm.DB, cache *redislib.Client) http.Handler {
	mux := http.NewServeMux()
	handler := probeHandler{cfg: cfg, db: db, cache: cache}
	mux.HandleFunc("/healthz", handler.healthz)
	mux.HandleFunc("/readyz", handler.readyz)
	return mux
}

func StartProbeServer(ctx context.Context, cfg config.Config, log *slog.Logger, db *gorm.DB, cache *redislib.Client) *http.Server {
	if cfg.HealthPort == "" {
		return nil
	}
	server := &http.Server{
		Addr:              ":" + cfg.HealthPort,
		Handler:           NewProbeHandler(cfg, db, cache),
		ReadHeaderTimeout: 3 * time.Second,
	}
	go func() {
		log.Info("worker probe server starting", slog.String("addr", server.Addr))
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("worker probe server failed", slog.String("error", err.Error()))
		}
	}()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Error("worker probe server shutdown failed", slog.String("error", err.Error()))
		}
	}()
	return server
}

func (h probeHandler) healthz(w http.ResponseWriter, r *http.Request) {
	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"service":     "final-review-worker",
		"environment": h.cfg.Environment,
		"ready":       true,
	})
}

func (h probeHandler) readyz(w http.ResponseWriter, r *http.Request) {
	dependencies := map[string]string{
		"postgres": h.postgresStatus(r.Context()),
		"redis":    h.redisStatus(r.Context()),
	}
	ready := dependencies["postgres"] == "ok" && dependencies["redis"] == "ok"
	status := http.StatusOK
	if !ready {
		status = http.StatusServiceUnavailable
	}
	h.writeJSON(w, status, map[string]interface{}{
		"service":      "final-review-worker",
		"environment":  h.cfg.Environment,
		"ready":        ready,
		"dependencies": dependencies,
	})
}

func (h probeHandler) postgresStatus(parent context.Context) string {
	if h.db == nil {
		return "not_configured"
	}
	sqlDB, err := h.db.DB()
	if err != nil {
		return "error"
	}
	ctx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		return "down"
	}
	return "ok"
}

func (h probeHandler) redisStatus(parent context.Context) string {
	if h.cache == nil {
		return "not_configured"
	}
	ctx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()
	if err := h.cache.Ping(ctx).Err(); err != nil {
		return "down"
	}
	return "ok"
}

func (h probeHandler) writeJSON(w http.ResponseWriter, status int, payload map[string]interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
