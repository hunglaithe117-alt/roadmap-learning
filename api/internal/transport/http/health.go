package httptransport

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// healthTimeout limits database ping duration during health checks.
const healthTimeout = 2 * time.Second

// Pinger defines the database ping capability required by the health handler.
type Pinger interface {
	PingContext(ctx context.Context) error
}

// healthHandler returns a Gin handler checking database connectivity and audio engine status.
func healthHandler(db Pinger, audio func() (string, bool), log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), healthTimeout)
		defer cancel()
		if db == nil {
			writeJSONError(c, http.StatusServiceUnavailable, "database not configured")
			return
		}
		if err := db.PingContext(ctx); err != nil {
			_ = c.Error(err)
			writeJSONError(c, http.StatusServiceUnavailable, "database not ready")
			return
		}
		status := "ok"
		if audio != nil {
			if name, real := audio(); !real {
				status = "degraded"
				log.Warn("audio using stub engine", slog.String("engine", name))
			}
		}
		c.JSON(http.StatusOK, gin.H{"status": status})
	}
}

