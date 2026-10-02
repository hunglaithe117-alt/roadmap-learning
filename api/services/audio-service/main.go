// Command audio-service là tiến trình RIÊNG xử lý TTS + STT cho app chính.
//
// Vì sao tách process: Piper là binary Python (~752MB model + runtime), Whisper
// là sidecar 8GB. Ở v1 chúng nằm trong main image / sidecar compose và app gọi
// bằng `os/exec` + HTTP client. M4 gom cả hai về sau 1 ranh giới gRPC duy nhất
// (`proto/audio/v1/audio.proto`) để app không còn phụ thuộc đường dẫn file
// tạm, không còn parse JSON của Whisper, và có healthcheck + timeout rõ ràng.
//
// Cấu hình qua env:
//   - PIPER_BIN, PIPER_MODEL_ZH, PIPER_MODEL_EN — TTS (rỗng = stub sine)
//   - WHISPER_URL                             — STT (rỗng = stub transcript)
//   - AUDIO_GRPC_ADDR                         — địa chỉ lắng nghe, mặc định :9090
//   - LOG_LEVEL                               — slog level
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
		log.Error("audio-service dừng vì lỗi", slog.String("err", err.Error()))
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	addr := envOr("AUDIO_GRPC_ADDR", ":9090")

	// TTS + STT dựng 1 lần, trước khi listen: nếu cấu hình sai (ví dụ
	// PIPER_BIN trỏ vào file không tồn tại) thì fail NGAY lúc boot thay vì
	// nhận request rồi mới trả 502 cho từng request.
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
		log.Info("audio-service lắng nghe", slog.String("addr", addr),
			slog.String("tts", synth.Name()), slog.String("stt", transcriber.Name()))
		if err := srv.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			return err
		}
		return nil
	})
	group.Go(func() error {
		<-groupCtx.Done()
		// `GracefulStop` chờ các RPC đang chạy xong; `Stop` cắt ngang. Chọn
		// graceful trước, ngắt sau `shutdownGrace` để 1 Whisper call chậm
		// không treo cả container.
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

// errEmptyAudio là lỗi nghiệp vụ duy nhất của service: audio 0 byte thì không
// có gì để nhận dạng. App map sang 400 (input sai) chứ không 502 — service
// trả `codes.InvalidArgument` để app đọc được mã.
var errEmptyAudio = status.Error(codes.InvalidArgument, "audio rỗng")

// shutdownGrace là khoảng chờ cho RPC đang chạy khi nhận SIGTERM, trước khi
// cắt ngang. 1 call Whisper có thể mất tới sttTimeout nên không thể đợi vô
// hạn — `docker compose stop` phải không treo.
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
