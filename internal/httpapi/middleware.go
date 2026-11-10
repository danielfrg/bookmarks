package httpapi

import (
	"encoding/json"
	"log"
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
var uploadPath = regexp.MustCompile(`^(/api/bookmarks/[0-9]+/archives|/api/import/netscape)$`)

const importPath = "/api/import/netscape"

const tokensPath = "/api/tokens"

// protectAPI guards /api/*. A request with a Bearer/Token Authorization header
// is an external client: the token must verify (401 otherwise, 503 when it
// cannot be checked) and no origin checks apply.
// Anything else is the web UI: reads are open, writes need the X-Bookmarks
// header, a same-host Origin (when sent) and a JSON body (multipart too on the
// archive upload route). Bodies are size limited.
func protectAPI(next http.Handler, tokens auth.Verifier, maxUploadBytes int64) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api" && !strings.HasPrefix(r.URL.Path, "/api/") {
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
			// Token management is browser-only: a leaked token must not be able
			// to mint replacements for itself or undo its revocation.
			if r.URL.Path == tokensPath || strings.HasPrefix(r.URL.Path, tokensPath+"/") {
				writeDetail(w, http.StatusForbidden, "API tokens are managed in the web UI")
				return
			}
			ok, err := tokens.VerifyToken(r.Context(), token)
			if err != nil {
				log.Printf("httpapi: verify API token: %v", err)
				writeDetail(w, http.StatusServiceUnavailable, "could not verify API token")
				return
			}
			if !ok {
				writeDetail(w, http.StatusUnauthorized, "invalid API token")
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

func writeDetail(w http.ResponseWriter, status int, detail string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"detail": detail})
}
