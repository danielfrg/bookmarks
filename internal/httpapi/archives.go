package httpapi

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/danielfrg/bookmarks/internal/bookmarks"
	"github.com/danielfrg/bookmarks/internal/service"
	"github.com/danielgtaylor/huma/v2"
)

type archiveJSON struct {
	ID          int64     `json:"id"`
	Source      string    `json:"source"`
	ContentType string    `json:"content_type"`
	DisplayName string    `json:"display_name"`
	FileSize    int64     `json:"file_size"`
	CreatedAt   time.Time `json:"created_at"`
	ViewURL     string    `json:"view_url"`
	DownloadURL string    `json:"download_url"`
}

func toArchive(a bookmarks.Archive) archiveJSON {
	return archiveJSON{
		ID: a.ID, Source: string(a.Source), ContentType: a.ContentType, DisplayName: a.DisplayName,
		FileSize: a.FileSize, CreatedAt: a.CreatedAt.UTC(),
		ViewURL: fmt.Sprintf("/_/archives/%d", a.ID), DownloadURL: fmt.Sprintf("/_/archives/%d/download", a.ID),
	}
}

type archiveListOutput struct {
	Body []archiveJSON
}

func registerArchives(api huma.API, svc *service.Service) {
	huma.Register(api, huma.Operation{OperationID: "list-archives", Method: http.MethodGet, Path: "/_/api/bookmarks/{id}/archives", Summary: "List the archives of a bookmark"},
		func(ctx context.Context, in *idInput) (*archiveListOutput, error) {
			list, err := svc.ListArchives(ctx, in.ID)
			if err != nil {
				return nil, apiError(err)
			}
			out := &archiveListOutput{Body: make([]archiveJSON, 0, len(list))}
			for _, a := range list {
				out.Body = append(out.Body, toArchive(a))
			}
			return out, nil
		})

	huma.Register(api, huma.Operation{OperationID: "delete-archive", Method: http.MethodDelete, Path: "/_/api/archives/{id}", Summary: "Delete an archive", DefaultStatus: http.StatusNoContent},
		func(ctx context.Context, in *idInput) (*emptyOutput, error) {
			if err := svc.DeleteArchive(ctx, in.ID); err != nil {
				return nil, apiError(err)
			}
			return &emptyOutput{}, nil
		})
}

// uploadArchive handles POST /_/api/bookmarks/{id}/archives (multipart).
//
// It is a plain handler instead of a Huma operation so the file part streams
// from the request to disk through r.MultipartReader (ParseMultipartForm would
// buffer the whole upload). Parts are read in order, so the `source` and
// `created_at` fields must come BEFORE the `file` part.
func uploadArchive(svc *service.Service) http.HandlerFunc {
	const orderMsg = "the source and created_at fields must come before the file part"
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id < 1 {
			writeProblem(w, http.StatusNotFound, "not found")
			return
		}
		mr, err := r.MultipartReader()
		if err != nil {
			writeProblem(w, http.StatusUnsupportedMediaType, "use multipart/form-data")
			return
		}
		in := service.ArchiveInput{Source: bookmarks.SourceUpload}
		var stored *bookmarks.Archive
		// Any return without a 201 removes an archive that was already stored,
		// so a failed upload leaves nothing behind (and a retry cannot duplicate).
		committed := false
		defer func() {
			if stored != nil && !committed {
				if err := svc.DeleteArchive(context.WithoutCancel(r.Context()), stored.ID); err != nil {
					log.Printf("httpapi: remove archive %d of failed upload: %v", stored.ID, err)
				}
			}
		}()
		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				break
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
			switch part.FormName() {
			case "source", "created_at":
				if stored != nil {
					// The file was already stored with default values; the
					// deferred cleanup removes it.
					writeProblem(w, http.StatusUnprocessableEntity, orderMsg)
					return
				}
				value, err := io.ReadAll(io.LimitReader(part, 256))
				if err != nil {
					writeError(w, err)
					return
				}
				if part.FormName() == "source" {
					in.Source = bookmarks.ArchiveSource(strings.TrimSpace(string(value)))
					continue
				}
				t, err := time.Parse(time.RFC3339, strings.TrimSpace(string(value)))
				if err != nil {
					writeProblem(w, http.StatusUnprocessableEntity, "created_at: must be an RFC 3339 timestamp")
					return
				}
				in.CreatedAt = &t
			case "file":
				if stored != nil {
					writeProblem(w, http.StatusUnprocessableEntity, "only one file part is allowed")
					return
				}
				a, err := addFilePart(r.Context(), svc, id, in, part)
				if err != nil {
					writeError(w, err)
					return
				}
				stored = &a
			}
		}
		if stored == nil {
			writeProblem(w, http.StatusUnprocessableEntity, "file: is required")
			return
		}
		committed = true
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(toArchive(*stored))
	}
}

func addFilePart(ctx context.Context, svc *service.Service, bookmarkID int64, in service.ArchiveInput, part *multipart.Part) (bookmarks.Archive, error) {
	body := bufio.NewReaderSize(part, 512)
	in.Body = body
	in.DisplayName = filepath.Base(strings.ReplaceAll(part.FileName(), `\`, "/"))
	if in.DisplayName == "." || in.DisplayName == "/" {
		in.DisplayName = ""
	}
	// A client that does not know the type (curl sends application/octet-stream)
	// gets sniffed from the first bytes.
	ct, _, _ := mime.ParseMediaType(part.Header.Get("Content-Type"))
	if ct == "" || ct == "application/octet-stream" {
		head, _ := body.Peek(512)
		ct = http.DetectContentType(head)
	}
	in.ContentType = ct
	return svc.AddArchive(ctx, bookmarkID, in)
}
