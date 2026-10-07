package httptransport

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	audiodomain "langapp/internal/domain/audio"
)

// ServerTimeouts defines timeout durations for the HTTP server.
type ServerTimeouts struct {
	ReadHeader time.Duration
	Read       time.Duration
	Write      time.Duration
	Idle       time.Duration
}

// DefaultServerTimeouts returns default timeout values for the HTTP server.
func DefaultServerTimeouts() ServerTimeouts {
	return ServerTimeouts{
		ReadHeader: 5 * time.Second,
		Read:       30 * time.Second,
		Write:      60 * time.Second,
		Idle:       120 * time.Second,
	}
}

// Options configures the HTTP router.
type Options struct {
	GinMode            string
	WebDist            string
	Dev                bool
	MaxMultipartMemory int64
	TTS                audiodomain.TTSSynthesizer
	STT                audiodomain.STTTranscriber
	GraphQL            http.Handler
	Playground         http.Handler
	DB                 Pinger
	Audio              func() (name string, real bool)
	Log                *slog.Logger
}

// Router wraps the configured Gin engine and logger.
type Router struct {
	Engine *gin.Engine
	Log    *slog.Logger
}

// NewRouter constructs a configured Gin router.
func NewRouter(opts Options) *Router {
	if opts.GinMode != "" {
		gin.SetMode(opts.GinMode)
	}
	log := opts.Log
	if log == nil {
		log = slog.Default()
	}
	if opts.MaxMultipartMemory <= 0 {
		opts.MaxMultipartMemory = defaultMaxMultipartMemory
	}

	e := gin.New()
	if err := e.SetTrustedProxies(nil); err != nil {
		log.Warn("failed to set trusted proxies", slog.String("err", err.Error()))
	}
	e.Use(corsMiddleware(opts.Dev))
	e.Use(gin.Recovery())
	e.Use(gin.LoggerWithConfig(gin.LoggerConfig{
		Output:    slogWriter{log: log},
		Formatter: compactLogFormatter,
		SkipPaths: []string{"/favicon.ico"},
	}))
	e.MaxMultipartMemory = opts.MaxMultipartMemory

	registerAPI(e, opts)
	if opts.GraphQL != nil {
		e.POST("/query", gin.WrapF(opts.GraphQL.ServeHTTP))
		e.GET("/query", gin.WrapF(opts.GraphQL.ServeHTTP))
	}
	if opts.Playground != nil {
		e.GET("/playground", gin.WrapH(opts.Playground))
	}
	if opts.WebDist == "" {
		e.NoRoute(func(c *gin.Context) {
			writeJSONError(c, http.StatusNotFound, "không tìm thấy "+c.Request.URL.Path)
		})
	} else {
		registerStatic(e, opts.WebDist, log)
	}
	return &Router{Engine: e, Log: log}
}

// Server constructs an http.Server configured with the given timeouts and base context.
func (r *Router) Server(baseCtx context.Context, t ServerTimeouts) *http.Server {
	return &http.Server{
		Addr:              addr(),
		Handler:           r.Engine,
		ReadHeaderTimeout: t.ReadHeader,
		ReadTimeout:       t.Read,
		WriteTimeout:      t.Write,
		IdleTimeout:       t.Idle,
		BaseContext:       func(net.Listener) context.Context { return baseCtx },
	}
}

