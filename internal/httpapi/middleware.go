package httpapi

import (
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/danielfrg/bookmarks/internal/auth"
)

const jsonBodyLimit = 1 << 20

// uploadPath matches the multipart routes: the archive upload and the
// Netscape import (whose file is capped at maxImportBytes by its handler).
var uploadPath = regexp.MustCompile(`^(/_/api/bookmarks/[0-9]+/archives|/_/api/import/netscape)$`)

const importPath = "/_/api/import/netscape"

// protectAPI guards /_/api/*. A request with an Authorization header is an
// external client and must carry the API token; it needs no origin checks.
// Anything else is the web UI: reads are open, writes need the X-Bookmarks
// header, a same-host Origin (when sent) and a JSON body (multipart too on the
// archive upload route). Bodies are size limited.
func protectAPI(next http.Handler, apiToken string, maxUploadBytes int64) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/_/api" && !strings.HasPrefix(r.URL.Path, "/_/api/") {
			next.ServeHTTP(w, r)
			return
		}
		upload := r.Method == http.MethodPost && uploadPath.MatchString(r.URL.Path)
		limit := int64(jsonBodyLimit)
		if upload {
			limit += maxUploadBytes
			if r.URL.Path == importPath {
				limit = jsonBodyLimit + maxImportBytes
			}
		}
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, limit)
		}

		// Only Bearer/Token schemes are API-token auth; others (e.g. Basic added
		// by a reverse proxy) are ignored and follow the browser path below.
		if token, ok := auth.TokenFromHeader(r.Header.Get("Authorization")); ok {
			if !auth.Equal(token, apiToken) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"detail":"invalid API token"}` + "\n"))
				return
			}
			next.ServeHTTP(w, r)
			return
		}

		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
			if r.Header.Get("X-Bookmarks") != "1" {
				http.Error(w, "X-Bookmarks header is required", http.StatusForbidden)
				return
			}
			if origin := r.Header.Get("Origin"); origin != "" {
				u, err := url.Parse(origin)
				if err != nil || u.Host != r.Host || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
					http.Error(w, "Origin does not match the request host", http.StatusForbidden)
					return
				}
			}
			// Bodyless writes (DELETE, POST .../capture) have no content type.
			if r.ContentLength != 0 {
				media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
				if err != nil || (media != "application/json" && !(upload && media == "multipart/form-data")) {
					http.Error(w, "Use application/json", http.StatusUnsupportedMediaType)
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}
