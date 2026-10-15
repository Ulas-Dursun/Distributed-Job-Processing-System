package handler

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

type HealthHandler struct {
	db    *sql.DB
	redis *redis.Client
}

func NewHealthHandler(db *sql.DB, redisClient *redis.Client) *HealthHandler {
	return &HealthHandler{
		db:    db,
		redis: redisClient,
	}
}

// Health is a simple check kept for backward compatibility.
func (h *HealthHandler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"service": "queueforge",
	})
}

// Live is the Kubernetes/Cloud Liveness probe (is the container process alive?).
func (h *HealthHandler) Live(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "alive",
	})
}

// Ready is the Kubernetes/Cloud Readiness probe (are dependencies healthy and accepting traffic?).
func (h *HealthHandler) Ready(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	dbStatus := "up"
	if h.db != nil {
		if err := h.db.PingContext(ctx); err != nil {
			dbStatus = "down"
		}
	}

	redisStatus := "up"
	if h.redis != nil {
		if err := h.redis.Ping(ctx).Err(); err != nil {
			redisStatus = "down"
		}
	}

	if dbStatus != "up" || redisStatus != "up" {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status":   "degraded",
			"database": dbStatus,
			"redis":    redisStatus,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":   "ready",
		"database": dbStatus,
		"redis":    redisStatus,
	})
}
