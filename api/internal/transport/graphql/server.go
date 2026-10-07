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

const (
	queryCacheSize  = 1000
	apqCacheSize    = 100
	complexityLimit = 200
	playgroundTTL   = 10 * time.Second
)

// Options configures the GraphQL server handler.
type Options struct {
	Introspection bool
	Dev           bool
	Log           *slog.Logger
}

// complexityErrorCode is the extension code for complexity limit errors.
const complexityErrorCode = "COMPLEXITY_LIMIT_EXCEEDED"

func init() {
	errcode.RegisterErrorType(complexityErrorCode, errcode.KindProtocol)
}

// NewServer constructs the GraphQL http.Handler with dataloader middleware.
func NewServer(r *Resolver, opts Options) http.Handler {
	log := opts.Log
	if log == nil {
		log = slog.Default()
	}
	srv := handler.New(generated.NewExecutableSchema(generated.Config{Resolvers: r}))

	srv.AddTransport(transport.POST{})
	srv.AddTransport(transport.GET{})

	srv.SetQueryCache(lru.New[*ast.QueryDocument](queryCacheSize))
	srv.Use(extension.AutomaticPersistedQuery{Cache: lru.New[string](apqCacheSize)})
	srv.Use(extension.FixedComplexityLimit(complexityLimit))
	if opts.Introspection {
		srv.Use(extension.Introspection{})
	}
	srv.SetErrorPresenter(errorPresenter(log))

	inner := srv
	return Middleware(r.Roadmap, r.SRS, inner)
}

// PlaygroundHandler returns the GraphiQL playground handler, or 404 if Dev is false.
func PlaygroundHandler(opts Options) http.Handler {
	if !opts.Dev {
		return http.NotFoundHandler()
	}
	return playground.Handler("langapp GraphQL", "/query")
}

// errorPresenter formats GraphQL errors and redacts internal errors.
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
