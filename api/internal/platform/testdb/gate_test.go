package testdb_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Test này chứng minh CƠ CHẾ gate thật sự hoạt động, chứ không chỉ "tin" rằng
// nó hoạt động.
//
// Vì sao cần: bug mà M7c vá là "0 FAIL trong khi 1/3 suite không chạy". Test
// viết kiểu "assert `SkipAllowed()` trả false" chỉ chứng minh hàm trả đúng
// giá trị — KHÔNG chứng minh `go test` thật sự exit ≠ 0. Đó chính là loại test
// tự lừa mình mà task này đang diệt.
//
// Nên test ở đây chạy `go test` như 1 SUBPROCESS thật, với env trống, rồi kiểm
// exit code + thông báo. Đây là hợp đồng quan sát được từ bên ngoài — cùng thứ
// CI/người khác nhìn thấy.
//
// Chạy subprocess nên test này chậm (~vài giây) và cần `go` trong PATH. Đó là
// đánh đổi đáng chấp nhận: nó là chốt chặn duy nhất chứng minh gate không bị
// gỡ âm thầm trong 1 refactor sau này.
func Test_missing_dsn_makes_the_suite_fail_not_skip(t *testing.T) {
	root := repoRoot(t)

	// Cố ý KHÔNG truyền LANGAPP_TEST_POSTGRES_DSN / ALLOW_SKIP_DB_TESTS.
	// `Env` chỉ giữ biến thật sự cần để `go` chạy được (PATH, HOME, cache) —
	// nếu để mặc định, `go test` có thể vô tình kế thừa DSN từ shell của dev
	// và test xanh vì lý do không liên quan tới cơ chế đang kiểm.
	cmd := exec.Command("go", "test", "-count=1", "./internal/infrastructure/srs/")
	cmd.Dir = root
	cmd.Env = minimalGoEnv(t)

	out, err := cmd.CombinedOutput()

	require.Error(t, err, "thiếu DSN mà `go test` vẫn exit 0 ⇒ gate đã bị gỡ hoặc vô hiệu")
	exitErr, ok := err.(*exec.ExitError)
	require.True(t, ok, "lỗi phải là exit code khác 0, không phải lỗi chạy `go`: %v", err)
	require.NotEqual(t, 0, exitErr.ExitCode())

	text := string(out)
	require.Contains(t, text, "LANGAPP_TEST_POSTGRES_DSN",
		"thông báo phải NÊU TÊN biến cần set — không nêu thì người đọc không biết phải làm gì")
	require.Contains(t, text, "ALLOW_SKIP_DB_TESTS",
		"thông báo phải nêu cả cửa thoát có chủ đích, để không ai tưởng không còn đường nào")
}

// ALLOW_SKIP_DB_TESTS=1 phải mở lại đường thoát: không có Postgres thì vẫn
// chạy được phần test không cần DB. Nếu cái này hỏng, mọi người sẽ tắt gate
// thay vì sửa hạ tầng — và ta lại quay về "suite im lặng".
func Test_allow_skip_env_reopens_the_escape_hatch(t *testing.T) {
	cmd := exec.Command("go", "test", "-count=1", "./internal/infrastructure/srs/")
	cmd.Dir = repoRoot(t)
	env := append(minimalGoEnv(t), "ALLOW_SKIP_DB_TESTS=1")
	cmd.Env = env

	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "ALLOW_SKIP_DB_TESTS=1 phải cho chạy: %s", out)
}

// DSN hợp lệ mà vẫn fail thì là lỗi thật (kết nối sai, migration hỏng…) —
// không liên quan tới gate. Chỉ assert là suite chạy được, không assert xanh:
// khi DSN trỏ nhầm database app thì `requireIsolatedDB` cố ý fail, và test này
// không nên phụ thuộc vào việc database đó đã được dựng hay chưa.
func Test_real_dsn_lets_the_suite_run(t *testing.T) {
	if os.Getenv("LANGAPP_TEST_POSTGRES_DSN") == "" {
		t.Skip("cần DSN thật để chạy test này — chạy qua `make test`")
	}
	cmd := exec.Command("go", "test", "-count=1", "-run", "TestALWAYS_MATCHES_NOTHING", "./internal/infrastructure/srs/")
	cmd.Dir = repoRoot(t)
	cmd.Env = append(minimalGoEnv(t),
		"LANGAPP_TEST_POSTGRES_DSN="+os.Getenv("LANGAPP_TEST_POSTGRES_DSN"))

	out, err := cmd.CombinedOutput()
	require.NoError(t, err,
		"DSN hợp lệ mà gate vẫn fail ⇒ gate chặn nhầm: %s", out)
}

// repoRoot tìm thư mục gốc module (`go.mod`) bằng cách đi ngược lên từ thư mục
// test hiện tại. Không hardcode đường dẫn tuyệt đối — test phải chạy được từ
// bất kỳ đâu trong repo (kể cả khi orchestrator chạy ở worktree khác).
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("không tìm thấy go.mod khi đi ngược từ thư mục test")
		}
		dir = parent
	}
}

// minimalGoEnv trả env tối thiểu cho `go test` chạy được mà KHÔNG kế thừa DSN
// từ môi trường của người chạy.
//
// Giữ: PATH (tìm `go`), HOME + GOCACHE/GOMODCACHE/GOPATH (build cache), và
// các biến proxy nếu có. Bỏ `LANGAPP_TEST_POSTGRES_DSN` + `ALLOW_SKIP_DB_TESTS`
// dù chúng có đang set hay không — đó mới là ý nghĩa của test này.
func minimalGoEnv(t *testing.T) []string {
	t.Helper()
	keep := []string{"PATH", "HOME", "GOCACHE", "GOMODCACHE", "GOPATH", "GOPROXY",
		"GOFLAGS", "GOROOT", "GOTOOLCHAIN", "CGO_ENABLED", "TMPDIR", "USER"}
	var env []string
	for _, k := range keep {
		if v, ok := os.LookupEnv(k); ok && v != "" {
			env = append(env, k+"="+v)
		}
	}
	// GOMODCACHE/GOCACHE có thể chưa set nhưng `go` vẫn cần biết chỗ để build.
	if !strings.Contains(strings.Join(env, " "), "GOCACHE=") {
		if v, err := exec.Command("go", "env", "GOCACHE").Output(); err == nil {
			env = append(env, "GOCACHE="+strings.TrimSpace(string(v)))
		}
	}
	return env
}
