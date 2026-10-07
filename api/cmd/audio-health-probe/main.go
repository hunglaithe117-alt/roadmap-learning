// Command audio-health-probe checks the gRPC health status of audio-service.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

// probeTimeout limits health check execution time.
const probeTimeout = 3 * time.Second

// defaultAddr is the fallback address for audio-service.
const defaultAddr = "127.0.0.1:9090"

func main() {
	addr := addrFrom(os.Args[1:], os.Getenv("AUDIO_GRPC_ADDR"))
	if err := check(addr, probeTimeout); err != nil {
		fmt.Fprintf(os.Stderr, "audio-health-probe: %s: %v\n", addr, err)
		os.Exit(1)
	}
	os.Exit(0)
}

// addrFrom determines the target address from arguments, environment, or default.
func addrFrom(args []string, env string) string {
	if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
		return normalize(args[0])
	}
	if env = strings.TrimSpace(env); env != "" {
		return normalize(env)
	}
	return defaultAddr
}

func normalize(addr string) string {
	if strings.HasPrefix(addr, ":") {
		return "127.0.0.1" + addr
	}
	return addr
}

// check executes a gRPC health check against the target address.
func check(addr string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()

	resp, err := healthpb.NewHealthClient(conn).Check(ctx, &healthpb.HealthCheckRequest{})
	if err != nil {
		return err
	}
	if resp.GetStatus() != healthpb.HealthCheckResponse_SERVING {
		return fmt.Errorf("status=%s", resp.GetStatus())
	}
	return nil
}
