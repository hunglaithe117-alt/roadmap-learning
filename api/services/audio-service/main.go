// Command audio-service provides gRPC TTS and STT services.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"

	audiopb "langapp/proto/audio/v1"
)

func main() {
	log := newLogger(os.Getenv("LOG_LEVEL"))
	if err := run(log); err != nil {
		log.Error("audio-service stopped with error", slog.String("err", err.Error()))
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	addr := envOr("AUDIO_GRPC_ADDR", ":9090")

	synth, err := newSynthesizerFromEnv(log)
	if err != nil {
		return err
	}
	transcriber, err := newTranscriberFromEnv(log)
	if err != nil {
		return err
	}

	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	srv := grpc.NewServer(
		grpc.MaxRecvMsgSize(maxAudioBytes),
		grpc.MaxSendMsgSize(maxAudioBytes),
	)
	audiopb.RegisterAudioServer(srv, &server{synth: synth, transcriber: transcriber, log: log})
	healthpb.RegisterHealthServer(srv, health.NewServer())
	reflection.Register(srv)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error {
		log.Info("audio-service listening", slog.String("addr", addr),
			slog.String("tts", synth.Name()), slog.String("stt", transcriber.Name()))
		if err := srv.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			return err
		}
		return nil
	})
	group.Go(func() error {
		<-groupCtx.Done()
		done := make(chan struct{})
		go func() { srv.GracefulStop(); close(done) }()
		select {
		case <-done:
		case <-time.After(shutdownGrace):
			srv.Stop()
		}
		return nil
	})
	return group.Wait()
}

// ErrEmptyAudio indicates the provided audio payload is empty.
var ErrEmptyAudio = status.Error(codes.InvalidArgument, "empty audio")

// shutdownGrace limits waiting time for in-flight RPCs during server termination.
const shutdownGrace = 10 * time.Second

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func newLogger(level string) *slog.Logger {
	var lv slog.Level
	switch level {
	case "debug":
		lv = slog.LevelDebug
	case "warn":
		lv = slog.LevelWarn
	case "error":
		lv = slog.LevelError
	default:
		lv = slog.LevelInfo
	}
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lv})
	l := slog.New(h)
	slog.SetDefault(l)
	return l
}
