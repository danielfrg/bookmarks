package httpapi

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/danielfrg/bookmarks/internal/service"
)

var fileName = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// archiveContentType is the Content-Type served for an archive.
func archiveContentType(ct string) string {
	if ct == "text/html" {
		return "text/html; charset=utf-8"
	}
	return ct
}

func archiveExt(ct string) string {
	switch ct {
	case "text/html", "application/xhtml+xml":
		return "html"
	case "application/pdf":
		return "pdf"
	}
	return "bin"
}

func archiveID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeProblem(w, http.StatusNotFound, "not found")
		return 0, false
	}
	return id, true
}

// viewArchive serves an archive inline in a sandbox.
func viewArchive(svc *service.Service, download bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := archiveID(w, r)
		if !ok {
			return
		}
		a, rc, err := svc.OpenArchive(r.Context(), id)
		if err != nil {
			writeError(w, err)
			return
		}
		defer rc.Close()
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Cache-Control", "private, max-age=3600")
		h.Set("Content-Type", archiveContentType(a.ContentType))
		if download {
			h.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="bookmark-%d-%d.%s"`, a.BookmarkID, a.ID, archiveExt(a.ContentType)))
			h.Set("Content-Security-Policy", "default-src 'none'; sandbox")
		} else {
			switch a.ContentType {
			case "text/html", "application/xhtml+xml":
				h.Set("Content-Security-Policy", "sandbox allow-scripts")
			case "application/pdf":
				h.Set("Content-Security-Policy", "default-src 'none'; object-src 'self'")
			default:
				h.Set("Content-Security-Policy", "default-src 'none'; sandbox")
			}
		}
		_, _ = io.Copy(w, rc)
	}
}

// serveImage serves a favicon or preview file from dir. The name must be a
// plain file name; anything else (or a missing file) is a 404.
func serveImage(dir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if !fileName.MatchString(name) || strings.HasPrefix(name, ".") {
			http.NotFound(w, r)
			return
		}
		f, err := os.Open(filepath.Join(dir, name))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()
		if st, err := f.Stat(); err != nil || !st.Mode().IsRegular() {
			http.NotFound(w, r)
			return
		}
		head := make([]byte, 512)
		n, _ := io.ReadFull(f, head)
		head = head[:n]
		ct := http.DetectContentType(head)
		if (strings.HasPrefix(ct, "text/xml") || strings.HasPrefix(ct, "text/plain")) && bytes.Contains(bytes.ToLower(head), []byte("<svg")) {
			ct = "image/svg+xml"
		}
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			http.NotFound(w, r)
			return
		}
		h := w.Header()
		h.Set("Content-Type", ct)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
		h.Set("Cache-Control", "public, max-age=86400")
		_, _ = io.Copy(w, f)
	}
}
