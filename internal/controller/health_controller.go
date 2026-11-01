package controller

import (
	"ai-audit-service/internal/pkg/response"
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type HealthController struct {
	db *pgxpool.Pool
}

func NewHealthController(db *pgxpool.Pool) *HealthController {
	return &HealthController{db: db}
}

func (h *HealthController) Check(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := h.db.Ping(ctx); err != nil {
		response.JSON(w, http.StatusServiceUnavailable, map[string]string{"status": "db_down"})
		return
	}

	response.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
