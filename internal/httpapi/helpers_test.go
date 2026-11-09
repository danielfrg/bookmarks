package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/danielfrg/bookmarks/internal/capture"
	"github.com/danielfrg/bookmarks/internal/fetch"
	"github.com/danielfrg/bookmarks/internal/jobs"
	"github.com/danielfrg/bookmarks/internal/service"
	"github.com/danielfrg/bookmarks/internal/store"
)

const testToken = "s3cret-token"

type noopQueue struct{}

func (noopQueue) Enqueue(context.Context, jobs.Kind, int64) error { return nil }

type fakeCapturer struct{}

func (fakeCapturer) Capture(context.Context, string) (capture.Result, error) {
	return capture.Result{}, nil
}

type testEnv struct {
	h   http.Handler
	dir string
	svc *service.Service
	st  *store.SQLite
}

// envOpts tweaks the test environment.
type envOpts struct {
	maxUpload int64 // service and handler upload limit; default 1 MiB
	noToken   bool
}

func newTestEnv(t *testing.T, opts ...func(*envOpts)) *testEnv {
	t.Helper()
	dir := t.TempDir()
	o := envOpts{maxUpload: 1 << 20}
	for _, f := range opts {
		f(&o)
	}
	st, err := store.Open(filepath.Join(dir, "bookmarks.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	svc := service.New(st, fetch.NewClient(fetch.Options{AllowPrivate: true}), fakeCapturer{}, noopQueue{}, service.Options{
		DataDir: dir, MaxUploadBytes: o.maxUpload, Logger: log.New(io.Discard, "", 0),
	})
	deps := Deps{Ping: st.Ping, Service: svc, APIToken: testToken, MaxUploadBytes: o.maxUpload, DataDir: dir}
	if o.noToken {
		deps.APIToken = ""
	}
	return &testEnv{h: NewHandler(fstest.MapFS{}, deps), dir: dir, svc: svc, st: st}
}

// do sends a request with the browser-style write headers (X-Bookmarks).
func (e *testEnv) do(method, path string, body any) *httptest.ResponseRecorder {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	r := httptest.NewRequest(method, path, rd)
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	if method != http.MethodGet {
		r.Header.Set("X-Bookmarks", "1")
	}
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, r)
	return w
}

func decode[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode %q: %v", w.Body.String(), err)
	}
	return v
}

type bookmarkResp struct {
	ID               int64    `json:"id"`
	URL              string   `json:"url"`
	Title            string   `json:"title"`
	Tags             []string `json:"tags"`
	Unread           bool     `json:"unread"`
	BasicCaptureOnly bool     `json:"basic_capture_only"`
	ReaderStatus     string   `json:"reader_status"`
	ArchiveCount     int      `json:"archive_count"`
	DateAdded        string   `json:"date_added"`
}

func (e *testEnv) create(t *testing.T, url string) bookmarkResp {
	t.Helper()
	w := e.do("POST", "/api/bookmarks?capture=none", map[string]any{"url": url, "title": "T"})
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	return decode[bookmarkResp](t, w)
}

func multipartBody(parts [][3]string) (string, *bytes.Buffer) {
	// each part: name, filename (empty for plain fields), content
	var buf bytes.Buffer
	const boundary = "XBOUNDARYX"
	for _, p := range parts {
		buf.WriteString("--" + boundary + "\r\n")
		if p[1] != "" {
			buf.WriteString(`Content-Disposition: form-data; name="` + p[0] + `"; filename="` + p[1] + `"` + "\r\n")
			if strings.HasSuffix(p[1], ".html") {
				buf.WriteString("Content-Type: text/html\r\n")
			}
		} else {
			buf.WriteString(`Content-Disposition: form-data; name="` + p[0] + `"` + "\r\n")
		}
		buf.WriteString("\r\n" + p[2] + "\r\n")
	}
	buf.WriteString("--" + boundary + "--\r\n")
	return "multipart/form-data; boundary=" + boundary, &buf
}
