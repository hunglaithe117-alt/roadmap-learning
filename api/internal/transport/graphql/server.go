package graphql

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/errcode"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/lru"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"

	"langapp/internal/transport/graphql/generated"
)

// Hằng cấu hình server. Ghi thành hằng thay vì magic number rải trong hàm để
// `docker-compose.yml` / DEPLOY.md đối chiếu được, và để test assert đúng con
// số đang chạy thay vì đoán.
const (
	// queryCacheSize là số operation đã phân tích giữ lại. Operation đã parse
	// xong không phụ thuộc dữ liệu, nên cache được an toàn. 1000 đủ cho app
	// single-user với ~30 operation.
	queryCacheSize = 1000
	// apqCacheSize là số persisted query id giữ. APQ cho phép client gửi
	// `extensions.persistedQuery.sha256Hash` thay vì toàn bộ text query — nhỏ
	// hơn nhiều trên request lặp lại.
	apqCacheSize = 100
	// complexityLimit là trần complexity của 1 operation. Vượt → HTTP 422
	// (đúng như yêu cầu của STACK-V2 §3).
	//
	// Vì sao 200 và không cao hơn: complexity của gqlgen = số field, và cây
	// roadmap hợp lệ nhất là 5 tầng × field. `paths { path { stages { topics {
	// resources { … } } } } }` đã ~40. 200 trần cho query hợp lệ có dư chỗ mà
	// chặn được query kiểu `a { b { c { … 20 tầng … } } } }` — vốn là cách duy
	// nhất để ép server quét cây 51 topic × 20 tầng.
	complexityLimit = 200
	// playgroundTTL là thời hạn cache của `GET /query` (đọc bằng query string
	// thay vì POST body). TTL ngắn vì query text nằm trong URL ⇒ nằm trong log
	// access. 10s là đủ để bấm link trong GraphiQL.
	playgroundTTL = 10 * time.Second
)

// Options là cấu hình dựng server, do `platform.Container` truyền xuống —
// transport KHÔNG tự đọc env, để `ENV=production` chỉ quyết định 1 chỗ.
type Options struct {
	// Introspection bật ở dev, TẮT ở production: để bật thì kẻ nào cũng đọc
	// được toàn bộ schema, đường dẫn fields, và tên type — đủ để dựng client
	// giả mạo. App single-user không có auth nên đây là biên tin cậy duy nhất.
	Introspection bool
	// Dev bật playground GraphiQL ở `/playground` và CORS `*` để Vite dev
	// (port khác) gọi được. Production tắt cả hai.
	Dev bool
	// Log nhận request GraphQL lỗi (5xx) và số complexity — không phải access
	// log của Gin, để 2 tầng không log trùng.
	Log *slog.Logger
}

// complexityErrorCode là mã extension mà `extension.FixedComplexityLimit` gắn
// vào lỗi khi vượt trần.
const complexityErrorCode = "COMPLEXITY_LIMIT_EXCEEDED"

func init() {
	// Mặc định của gqlgen, lỗi này trả HTTP **200** kèm `errors[]` — vì
	// `errcode.GetErrorKind` chỉ coi lỗi là "protocol" (⇒ 422) khi mã của nó có
	// trong bảng đăng ký, mà `COMPLEXITY_LIMIT_EXCEEDED` thì không.
	//
	// STACK-V2-PLAN §3 chốt "trả HTTP 422 khi vượt" ⇒ phải đăng ký. Không đăng
	// ký thì client không có tín hiệu nào để biết "query của tôi quá nặng" —
	// nó chỉ thấy 1 response 200 có lỗi, dễ xử lý như lỗi dữ liệu.
	//
	// `init` chạy 1 lần cho cả process nên không cần khoá.
	errcode.RegisterErrorType(complexityErrorCode, errcode.KindProtocol)
}

// NewServer dựng `http.Handler` xử lý GraphQL, đã bọc sẵn middleware dataloader.
//
// Trả về `http.Handler` chứ không phải `*handler.Server` để `internal/transport/
// http` mount bằng `gin.WrapF` mà không phải biết gqlgen — đúng ranh giới tầng.
func NewServer(r *Resolver, opts Options) http.Handler {
	log := opts.Log
	if log == nil {
		log = slog.Default()
	}
	srv := handler.New(generated.NewExecutableSchema(generated.Config{Resolvers: r}))

	// 2 transport: POST cho app, GET cho link mở trực tiếp / query có cache ngắn.
	srv.AddTransport(transport.POST{})
	srv.AddTransport(transport.GET{})

	// APQ trước cache query: APQ là bước rẻ nhất (tra sha256), và khi client
	// dùng APQ thì không cần parse lại operation.
	srv.SetQueryCache(lru.New[*ast.QueryDocument](queryCacheSize))
	srv.Use(extension.AutomaticPersistedQuery{Cache: lru.New[string](apqCacheSize)})
	srv.Use(extension.FixedComplexityLimit(complexityLimit))
	// Thứ tự: FixedComplexityLimit phải chạy SAU APQ (APQ lấy query từ cache
	// rồi mới đo complexity được) — `Use` chạy đúng thứ tự khai báo.
	//
	// `extension.Introspection` là struct rỗng KHÔNG có field bật/tắt: muốn
	// tắt thì không `Use` nó. Đây là điểm dễ làm ngược — thêm
	// `extension.Introspection{Introspection: false}` sẽ **bật** introspection,
	// đúng thứ ta cần tắt ở production.
	if opts.Introspection {
		srv.Use(extension.Introspection{})
	}
	srv.SetErrorPresenter(errorPresenter(log))

	inner := srv
	return Middleware(r.Roadmap, r.SRS, inner)
}

// PlaygroundHandler trả handler GraphiQL cho `GET /playground`, hoặc 404 khi
// `Dev = false`.
//
// Điều kiện `Dev` là bắt buộc chứ không phải tiện: GraphiQL là 1 form gửi query
// tùy ý — bật ở production tức là mở cửa introspection + query tùy ý cho ai có
// mạng nội bộ.
func PlaygroundHandler(opts Options) http.Handler {
	if !opts.Dev {
		return http.NotFoundHandler()
	}
	return playground.Handler("langapp GraphQL", "/query")
}

// errorPresenter thay `gqlerror.DefaultErrorPresenter` để log lỗi server-side
// và KHÔNG rò chi tiết lỗi hệ thống ra client.
//
// Phân loại 2 nhóm:
//   - `*gqlerror.Error` — do CHÍNH gqlgen sinh ra (validation, introspection bị
//     tắt, complexity vượt trần). Message đã an toàn để đưa ra ngoài (viết tay
//     trong thư viện, không chứa SQL/tên bảng) và là thứ client CẦN để hiểu vì
//     sao hỏng — thay cả bằng "lỗi hệ thống" thì query sai báo 500 và mất
//     thông tin.
//   - mọi thứ khác — lỗi Go thô từ resolver, có thể là lỗi GORM/Postgres chứa
//     câu SQL và tên bảng. Chỉ log server-side, trả message chung.
func errorPresenter(log *slog.Logger) graphql.ErrorPresenterFunc {
	return func(ctx context.Context, e error) *gqlerror.Error {
		var gqlErr *gqlerror.Error
		if errors.As(e, &gqlErr) {
			if gqlErr.Path != nil || gqlErr.Extensions != nil {
				log.WarnContext(ctx, "lỗi GraphQL ở tầng protocol", "err", gqlErr.Message)
			}
			return gqlErr
		}
		log.ErrorContext(ctx, "lỗi GraphQL không phân loại", "err", e.Error())
		return gqlerror.Errorf("lỗi hệ thống")
	}
}
