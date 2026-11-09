package httpapi

import (
	"context"
	"io"
	"io/fs"
	"net/http"
	"path/filepath"

	"github.com/danielfrg/bookmarks/internal/mcp"
	"github.com/danielfrg/bookmarks/internal/service"
	"github.com/danielfrg/bookmarks/internal/web"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
)

// Deps are the dependencies of the HTTP handler. MCPAllowedHosts extends the hosts
// the MCP endpoint accepts.
type Deps struct {
	Ping            func(context.Context) error
	Service         *service.Service
	APIToken        string
	MCPAllowedHosts []string
	MaxUploadBytes  int64
	DataDir         string // favicons and previews live in DataDir/favicons and DataDir/previews
}

func NewHandler(assets fs.FS, deps Deps) http.Handler {
	mux := http.NewServeMux()
	config := huma.DefaultConfig("Bookmarks API", "0.1.0")
	config.OpenAPIPath = "/_/api/openapi"
	config.DocsPath = "/_/api/docs"
	config.SchemasPath = ""
	config.CreateHooks = nil
	api := humago.New(mux, config)
	registerBookmarks(api, deps.Service)
	registerArchives(api, deps.Service)
	registerTags(api, deps.Service)
	registerBundles(api, deps.Service)
	registerSettings(api, deps.Service)
	registerBulk(api, deps.Service)
	mux.HandleFunc("POST /_/api/import/netscape", importNetscape(deps.Service))
	mux.HandleFunc("GET /_/api/export/netscape", exportNetscape(deps.Service))
	mux.HandleFunc("POST /_/api/bookmarks/{id}/archives", uploadArchive(deps.Service))
	mux.HandleFunc("GET /_/archives/{id}", viewArchive(deps.Service, false))
	mux.HandleFunc("GET /_/archives/{id}/download", viewArchive(deps.Service, true))
	mux.HandleFunc("GET /_/favicons/{name}", serveImage(filepath.Join(deps.DataDir, "favicons")))
	mux.HandleFunc("GET /_/previews/{name}", serveImage(filepath.Join(deps.DataDir, "previews")))
	mux.HandleFunc("GET /_/healthz", func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "ok\n") })
	mux.HandleFunc("GET /_/readyz", func(w http.ResponseWriter, r *http.Request) {
		if err := deps.Ping(r.Context()); err != nil {
			http.Error(w, "Database unavailable", http.StatusServiceUnavailable)
			return
		}
		_, _ = io.WriteString(w, "ok\n")
	})
	mux.Handle("/_/mcp", mcp.NewHandler(deps.Service, deps.MCPAllowedHosts, deps.APIToken))
	mux.Handle("GET /_/assets/", web.Handler(assets))
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) { web.Index(w, r, assets, http.StatusOK) })
	guarded := protectAPI(mux, deps.APIToken, deps.MaxUploadBytes)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Referrer-Policy", "no-referrer")
		if r.URL.Path == "/_/healthz" || r.URL.Path == "/_/readyz" {
			w.Header().Set("Cache-Control", "no-store")
		}
		guarded.ServeHTTP(w, r)
	})
}
