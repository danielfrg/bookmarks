package httpapi

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/danielfrg/bookmarks/internal/archive"
	"github.com/danielfrg/bookmarks/internal/service"
	"github.com/danielfrg/bookmarks/internal/store"
	"github.com/danielgtaylor/huma/v2"
)

// classify maps a service or store error to an HTTP status and a message that
// is safe to show to clients (never paths or SQL). Unknown errors are logged
// and reported as a generic 500.
func classify(err error) (int, string) {
	var ve *service.ValidationError
	var mbe *http.MaxBytesError
	switch {
	case errors.Is(err, store.ErrNotFound):
		return http.StatusNotFound, "not found"
	case errors.Is(err, store.ErrConflict):
		return http.StatusConflict, "a bookmark with this URL already exists"
	case errors.As(err, &ve):
		return http.StatusUnprocessableEntity, ve.Error()
	case errors.Is(err, archive.ErrTooLarge), errors.As(err, &mbe):
		return http.StatusRequestEntityTooLarge, "upload is too large"
	}
	log.Printf("httpapi: internal error: %v", err)
	return http.StatusInternalServerError, "internal server error"
}

// apiError converts err into a Huma error (problem+json).
func apiError(err error) error {
	status, msg := classify(err)
	return huma.NewError(status, msg)
}

// writeError writes err as problem+json for plain (non-Huma) handlers.
func writeError(w http.ResponseWriter, err error) {
	status, msg := classify(err)
	writeProblem(w, status, msg)
}

func writeProblem(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"title": http.StatusText(status), "status": status, "detail": msg})
}
