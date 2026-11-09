package httpapi

import (
	"context"
	"io"
	"io/fs"
	"net/http"

	"github.com/danielfrg/bookmarks/internal/web"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
)

type Deps struct {
	Ping func(context.Context) error
}

func NewHandler(assets fs.FS, deps Deps) http.Handler {
	mux := http.NewServeMux()
	config := huma.DefaultConfig("Bookmarks API", "0.1.0")
	config.OpenAPIPath = "/_/api/openapi"
	config.DocsPath = "/_/api/docs"
	config.SchemasPath = ""
	config.CreateHooks = nil
	humago.New(mux, config)
	mux.HandleFunc("GET /_/healthz", func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "ok\n") })
	mux.HandleFunc("GET /_/readyz", func(w http.ResponseWriter, r *http.Request) {
		if err := deps.Ping(r.Context()); err != nil {
			http.Error(w, "Database unavailable", http.StatusServiceUnavailable)
			return
		}
		_, _ = io.WriteString(w, "ok\n")
	})
	mux.Handle("GET /_/assets/", web.Handler(assets))
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) { web.Index(w, r, assets, http.StatusOK) })
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Referrer-Policy", "no-referrer")
		if r.URL.Path == "/_/healthz" || r.URL.Path == "/_/readyz" {
			w.Header().Set("Cache-Control", "no-store")
		}
		mux.ServeHTTP(w, r)
	})
}
