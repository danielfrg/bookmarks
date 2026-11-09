package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/danielfrg/bookmarks/internal/service"
)

// maxImportBytes caps the Netscape file of an import.
const maxImportBytes = 16 << 20

// importNetscape handles POST /_/api/import/netscape (multipart, part "file").
// Like the archive upload it streams the file part (no ParseMultipartForm), so
// the HTML is tokenized while it is received.
func importNetscape(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mr, err := r.MultipartReader()
		if err != nil {
			writeProblem(w, http.StatusUnsupportedMediaType, "use multipart/form-data")
			return
		}
		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				writeProblem(w, http.StatusUnprocessableEntity, "file: is required")
				return
			}
			if err != nil {
				var mbe *http.MaxBytesError
				if errors.As(err, &mbe) {
					writeError(w, err)
					return
				}
				writeProblem(w, http.StatusBadRequest, "malformed multipart body")
				return
			}
			if part.FormName() != "file" {
				continue
			}
			res, err := svc.ImportNetscape(r.Context(), http.MaxBytesReader(w, part, maxImportBytes))
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(res)
			return
		}
	}
}

// exportNetscape handles GET /_/api/export/netscape.
func exportNetscape(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="bookmarks.html"`)
		if err := svc.ExportNetscape(r.Context(), w); err != nil {
			// The body is already streaming, so only log (classify does).
			classify(err)
		}
	}
}
