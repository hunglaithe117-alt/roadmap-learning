package graphql_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	gqltransport "langapp/internal/transport/graphql"
)

// Introspection tắt ở production là biên tin cậy DUY NHẤT của app (v1 không có
// auth). Bật introspection ở production là mở cửa: kẻ nào đọc được schema thì
// dựng được client giả mạo gọi mọi mutation.
func Test_introspection_is_rejected_when_disabled(t *testing.T) {
	h := newHarness(t)
	// `Post` (không phải `MustPost`) vì ở production introspection BỊ TỪ CHỐI —
	// đó chính là điều test muốn khẳng định.
	err := h.prodClient(t).Post(`{ __schema { queryType { name } } }`, &struct{}{})
	require.Error(t, err, "introspection phải bị từ chối ở production")
	require.Contains(t, err.Error(), "introspection",
		"thông báo lỗi phải NÓI LÌ do là introspection, không phải lỗi chung chung: %v", err)
}

func Test_introspection_works_when_enabled_for_dev(t *testing.T) {
	h := newHarness(t)
	var resp struct {
		Schema struct {
			QueryType struct{ Name string } `json:"queryType"`
		} `json:"__schema"`
	}
	h.client(t).MustPost(`{ __schema { queryType { name } } }`, &resp)

	require.Equal(t, "Query", resp.Schema.QueryType.Name)
}

// Complexity > 200 phải trả HTTP 422. Đây là hàng rào chống query kiểu
// `a { b { c { … 20 tầng … } } } }` — thứ mà `FixedComplexityLimit` chặn được còn
// depth limit thì không (gqlgen không có sẵn).
func Test_query_over_complexity_limit_returns_422(t *testing.T) {
	h := newHarness(t)
	srv := httptest.NewServer(gqltransport.NewServer(h.containerResolver(), gqltransport.Options{
		Introspection: false, Dev: false, Log: testLogger(),
	}))
	t.Cleanup(srv.Close)

	// 201 field ⇒ complexity 201 > 200.
	fields := strings.TrimSuffix(strings.Repeat("streak ", 201), " ")
	q := "{ " + fields + " }"

	resp, err := http.Post(srv.URL+"/query", "application/json",
		strings.NewReader(`{"query":`+quote(q)+`}`))
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode,
		"query vượt complexity limit phải trả 422 (STACK-V2 §3)")
}

func Test_query_under_complexity_limit_is_accepted(t *testing.T) {
	h := newHarness(t)
	srv := httptest.NewServer(gqltransport.NewServer(h.containerResolver(), gqltransport.Options{
		Introspection: false, Dev: false, Log: testLogger(),
	}))
	t.Cleanup(srv.Close)

	// 199 field ⇒ dưới trần.
	fields := strings.TrimSuffix(strings.Repeat("streak ", 199), " ")
	q := "{ " + fields + " }"

	resp, err := http.Post(srv.URL+"/query", "application/json",
		strings.NewReader(`{"query":`+quote(q)+`}`))
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode, "query dưới trần phải qua")
}

// GET /query phải chạy được: client hay mở link debug bằng query string, và
// STACK-V2 §3 chốt mount cả 2 method.
func Test_get_query_endpoint_serves_operations(t *testing.T) {
	h := newHarness(t)
	srv := httptest.NewServer(gqltransport.NewServer(h.containerResolver(), gqltransport.Options{
		Introspection: true, Dev: true, Log: testLogger(),
	}))
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/query?query=" + urlEncode(`{ streak }`))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

// GET /playground chỉ có ở dev. Ở production nó phải 404 — GraphiQL là form gửi
// query tùy ý, bật ở production là mở đường vòng qua việc tắt introspection.
func Test_playground_is_404_in_production(t *testing.T) {
	srv := httptest.NewServer(gqltransport.PlaygroundHandler(gqltransport.Options{Dev: false}))
	t.Cleanup(srv.Close)
	resp, err := http.Get(srv.URL + "/playground")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func Test_playground_serves_graphiql_in_dev(t *testing.T) {
	srv := httptest.NewServer(gqltransport.PlaygroundHandler(gqltransport.Options{Dev: true}))
	t.Cleanup(srv.Close)
	resp, err := http.Get(srv.URL + "/playground")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

// quote là JSON-escape tối thiểu cho query test — đủ cho dấu `\` và `"`, không
// cài dependency.
func quote(s string) string {
	return `"` + strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), `"`, `\"`) + `"`
}

func urlEncode(s string) string { return url.QueryEscape(s) }
