package handlers

import (
	"bullet-commerce/internal/webutils"
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	serviceName    = "bullet-commerce"
	serviceVersion = "1.0.0"
)

type DBPinger interface {
	Ping(ctx context.Context) error
}

// HealthInfo reports optional-capability state on the readiness probe. These are
// informational and never gate readiness: payment and the AI assistant are optional
// and config-gated, so "disabled" is a valid steady state, not a failure.
type HealthInfo struct {
	PaymentProvider   string
	PaymentConfigured bool
	AIEnabled         bool
}

type HealthHandler struct {
	db   DBPinger
	info HealthInfo
}

func NewHealthHandler(db *pgxpool.Pool, info HealthInfo) *HealthHandler {
	return &HealthHandler{db: db, info: info}
}

func NewHealthHandlerWithPinger(p DBPinger, info HealthInfo) *HealthHandler {
	return &HealthHandler{db: p, info: info}
}

// Liveness answers "is the process alive?" and MUST NOT touch dependencies. If a DB
// or PSP outage failed here, the orchestrator would restart the container, turning a
// dependency blip into a reconnect storm that deepens the incident.
func (h *HealthHandler) Liveness(w http.ResponseWriter, r *http.Request) {
	webutils.WriteJSON(w, http.StatusOK, map[string]any{
		"status":    "ok",
		"service":   serviceName,
		"version":   serviceVersion,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// Readiness answers "should traffic route here?": it gates on the hard dependency
// (Postgres) and reports the optional capabilities for observability. Returns 503
// when the DB is down so the load balancer drains this instance.
func (h *HealthHandler) Readiness(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	start := time.Now()
	dbErr := h.db.Ping(ctx)
	db := map[string]any{
		"status":     "ok",
		"latency_ms": time.Since(start).Milliseconds(),
	}
	status := http.StatusOK
	overall := "ok"
	if dbErr != nil {
		db["status"] = "error"
		db["error"] = "database unavailable"
		status = http.StatusServiceUnavailable
		overall = "degraded"
	}

	// Optional capabilities: "disabled" when not configured is expected, not a fault.
	payment := "disabled"
	if h.info.PaymentConfigured {
		payment = "configured"
	}
	ai := "disabled"
	if h.info.AIEnabled {
		ai = "enabled"
	}

	webutils.WriteJSON(w, status, map[string]any{
		"status":    overall,
		"service":   serviceName,
		"version":   serviceVersion,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"checks": map[string]any{
			"database":     db,
			"payment":      map[string]any{"status": payment, "provider": h.info.PaymentProvider},
			"ai_assistant": map[string]any{"status": ai},
		},
	})
}
