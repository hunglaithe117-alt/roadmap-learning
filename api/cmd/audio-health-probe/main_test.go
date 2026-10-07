package main

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

// startHealthServer dựng gRPC server thật với health server, y hệt cách
// `api/services/audio-service/main.go` đăng ký. Dùng server thật (không mock)
// vì healthcheck đúng/sai là câu hỏi về hành vi thật của client.
//
// `setStatus == ""` ⇒ không gọi `SetServingStatus`, tức để nguyên trạng thái
// mặc định của `health.NewServer()`.
func startHealthServer(t *testing.T, setStatus healthpb.HealthCheckResponse_ServingStatus) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	srv := grpc.NewServer()
	hs := health.NewServer()
	if setStatus.String() != "" {
		hs.SetServingStatus("", setStatus)
	}
	healthpb.RegisterHealthServer(srv, hs)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	return lis.Addr().String()
}

func TestCheck_ServingIsHealthy(t *testing.T) {
	addr := startHealthServer(t, healthpb.HealthCheckResponse_SERVING)
	require.NoError(t, check(addr, 3*time.Second))
}

func TestCheck_NotServingIsUnhealthy(t *testing.T) {
	// Service còn sống nhưng tự báo chưa sẵn sàng. Probe phải ĐỎ — nếu chỉ
	// kiểm "port còn mở" thì lọt, đúng loại báo động giả đang chữa.
	addr := startHealthServer(t, healthpb.HealthCheckResponse_NOT_SERVING)
	require.Error(t, check(addr, 3*time.Second))
}

func TestCheck_UnknownIsUnhealthy(t *testing.T) {
	// Chưa ai set status ⇒ `SERVING_STATUS_UNKNOWN`. Cố ý KHÔNG tính là khoẻ:
	// "không ai set" ≠ "sẵn sàng". Đây là biến thể của bug gốc (coi mọi
	// phản hồi 200 là xanh).
	addr := startHealthServer(t, healthpb.HealthCheckResponse_UNKNOWN)
	require.Error(t, check(addr, 3*time.Second))
}

func TestCheck_DeadServerFails(t *testing.T) {
	// Cổng không có listener: probe phải đỏ TRONG timeout chứ không treo.
	start := time.Now()
	require.Error(t, check("127.0.0.1:1", 2*time.Second))
	require.Less(t, time.Since(start), 5*time.Second)
}

func TestCheck_ShortTimeoutFails(t *testing.T) {
	// Deadline phải được tôn trọng, nếu không container treo hết cửa sổ
	// healthcheck mỗi lần.
	addr := startHealthServer(t, healthpb.HealthCheckResponse_SERVING)
	require.Error(t, check(addr, time.Nanosecond))
}

func TestAddrFrom(t *testing.T) {
	cases := []struct {
		name string
		args []string
		env  string
		want string
	}{
		{"không có gì thì dùng default", nil, "", defaultAddr},
		{"env dạng listen `:9090` phải thành 127.0.0.1", nil, ":9090", "127.0.0.1:9090"},
		{"env đã có host", nil, "audio-service:9090", "audio-service:9090"},
		{"tham số dòng lệnh thắng env", []string{":9091"}, ":9090", "127.0.0.1:9091"},
		{"env toàn khoảng trắng thì rơi về default", nil, "   ", defaultAddr},
		{"tham số rỗng thì dùng env", []string{""}, "127.0.0.1:9090", "127.0.0.1:9090"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, addrFrom(tc.args, tc.env))
		})
	}
}
