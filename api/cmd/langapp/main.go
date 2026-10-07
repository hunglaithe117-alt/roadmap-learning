// Command langapp runs the HTTP and GraphQL server.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"langapp/internal/platform"
	gqltransport "langapp/internal/transport/graphql"
	httptransport "langapp/internal/transport/http"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx); err != nil {
		slog.Error("langapp stopped with error", slog.String("err", err.Error()))
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	c, err := platform.Build(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := c.Close(); cerr != nil && !errors.Is(cerr, http.ErrServerClosed) {
			c.Logger.Error("failed to close connections", slog.String("err", cerr.Error()))
		}
	}()

	if err := c.BootStep(ctx, "seed roadmap", c.Seed); err != nil {
		c.Logger.Warn("roadmap seed failed, continuing with existing data",
			slog.String("err", err.Error()))
	}

	srv, err := buildHTTP(ctx, c)
	if err != nil {
		return err
	}

	serveErr := make(chan error, 1)
	go func() {
		c.Logger.Info("langapp listening",
			slog.String("addr", srv.Addr),
			slog.String("gin_mode", c.Config.GinMode),
			slog.String("audio", c.Audio.EngineInfo().Name),
			slog.Bool("audio_real", c.Audio.IsReal()),
			slog.String("grpc_audio", c.Audio.Address),
			slog.Int64("goose_version", c.Migration.Version))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
			return
		}
		serveErr <- nil
	}()

	select {
	case err := <-serveErr:
		if err != nil {
			return fmt.Errorf("http server: %w", err)
		}
		return nil
	case <-ctx.Done():
		c.Logger.Info("shutdown signal received, terminating")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			c.Logger.Warn("shutdown timed out with active requests",
				slog.String("err", err.Error()))
		}
		<-serveErr
		c.Logger.Info("stopped")
		return nil
	}
}

// shutdownGrace limits waiting time for in-flight requests during server shutdown.
const shutdownGrace = 15 * time.Second

// buildHTTP constructs the configured http.Server instance.
func buildHTTP(ctx context.Context, c *platform.Container) (*http.Server, error) {
	sqlDB, err := c.SQLDB()
	if err != nil {
		return nil, err
	}

	resolver := gqltransport.NewResolver(c.SRS, c.Content, c.Roadmap, c.Practice, c.Insight, c.Sync)
	dev := c.Config.GinMode != "release"
	gqlOpts := gqltransport.Options{Introspection: dev, Dev: dev, Log: c.Logger}
	router := httptransport.NewRouter(httptransport.Options{
		GinMode:    c.Config.GinMode,
		WebDist:    webDist(),
		Dev:        dev,
		TTS:        c.Audio.TTSSynth,
		STT:        c.Audio.STTSTT,
		GraphQL:    gqltransport.NewServer(resolver, gqlOpts),
		Playground: gqltransport.PlaygroundHandler(gqlOpts),
		DB:         sqlDB,
		Audio: func() (string, bool) {
			info := c.Audio.TTSEngine()
			return info.Name, c.Audio.IsReal()
		},
		Log:                c.Logger,
		MaxMultipartMemory: 0,
	})

	return router.Server(ctx, httptransport.DefaultServerTimeouts()), nil
}

// webDist returns the configured web static assets directory path.
func webDist() string { return os.Getenv("WEB_DIST") }

