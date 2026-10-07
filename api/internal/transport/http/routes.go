package httptransport

import (
	"github.com/gin-gonic/gin"
)

// registerAPI registers REST and health check endpoints under the /api group.
func registerAPI(e *gin.Engine, opts Options) {
	api := e.Group("/api")
	api.GET("/health", healthHandler(opts.DB, opts.Audio, opts.Log))
	api.GET("/tts", ttsHandler(opts.TTS, opts.Log))
	api.POST("/stt", sttHandler(opts.STT, opts.Log))
}

