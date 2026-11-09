package web

import (
	"io/fs"
	"net/http"
	"strings"
)

func Index(w http.ResponseWriter, r *http.Request, assets fs.FS, status int) {
	data, err := fs.ReadFile(assets, "index.html")
	if err != nil {
		http.Error(w, "Frontend is not built. Run just frontend for development or just build for the embedded app.", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(status)
	if r.Method != http.MethodHead {
		_, _ = w.Write(data)
	}
}
func Handler(assets fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			Index(w, r, assets, 200)
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/_/assets/") {
			http.NotFound(w, r)
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/_/assets/")
		for _, part := range strings.Split(name, "/") {
			if strings.HasPrefix(part, ".") {
				http.NotFound(w, r)
				return
			}
		}
		info, err := fs.Stat(assets, name)
		if err != nil || !info.Mode().IsRegular() {
			http.NotFound(w, r)
			return
		}
		clone := r.Clone(r.Context())
		u := *r.URL
		u.Path = "/" + name
		u.RawPath = ""
		clone.URL = &u
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		http.FileServerFS(assets).ServeHTTP(w, clone)
	})
}
