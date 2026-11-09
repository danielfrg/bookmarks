package httpapi

import (
	"context"
	"fmt"
	"io/fs"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/danielfrg/bookmarks/internal/store"
)

type archiveResp struct {
	ID          int64  `json:"id"`
	Source      string `json:"source"`
	ContentType string `json:"content_type"`
	DisplayName string `json:"display_name"`
	FileSize    int64  `json:"file_size"`
	CreatedAt   string `json:"created_at"`
	ViewURL     string `json:"view_url"`
	DownloadURL string `json:"download_url"`
}

func (e *testEnv) upload(path, ctype string, body *strings.Reader, bearer bool) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", path, body)
	r.Header.Set("Content-Type", ctype)
	if bearer {
		r.Header.Set("Authorization", "Bearer "+testToken)
	} else {
		r.Header.Set("X-Bookmarks", "1")
	}
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, r)
	return w
}

func TestUploadArchiveAndView(t *testing.T) {
	e := newTestEnv(t)
	b := e.create(t, "https://example.com/a")
	html := "<html><body>hello <script>1</script></body></html>"
	ctype, body := multipartBody([][3]string{
		{"source", "", "extension"},
		{"created_at", "", "2026-01-02T03:04:05Z"},
		{"file", "page.html", html},
	})
	path := fmt.Sprintf("/api/bookmarks/%d/archives", b.ID)
	w := e.upload(path, ctype, strings.NewReader(body.String()), false)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	a := decode[archiveResp](t, w)
	if a.Source != "extension" || a.ContentType != "text/html" || a.DisplayName != "page.html" || a.FileSize != int64(len(html)) ||
		a.CreatedAt != "2026-01-02T03:04:05Z" ||
		a.ViewURL != fmt.Sprintf("/archives/%d", a.ID) || a.DownloadURL != fmt.Sprintf("/archives/%d/download", a.ID) {
		t.Fatalf("%+v", a)
	}

	w = e.do("GET", a.ViewURL, nil)
	if w.Code != 200 || w.Body.String() != html {
		t.Fatal(w.Code, w.Body.String())
	}
	if got := w.Header().Get("Content-Security-Policy"); got != "sandbox allow-scripts" {
		t.Fatalf("csp = %q", got)
	}
	if w.Header().Get("X-Content-Type-Options") != "nosniff" || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("headers = %v", w.Header())
	}
	w = e.do("GET", a.DownloadURL, nil)
	if w.Code != 200 || w.Body.String() != html {
		t.Fatal(w.Code)
	}
	want := fmt.Sprintf(`attachment; filename="bookmark-%d-%d.html"`, b.ID, a.ID)
	if got := w.Header().Get("Content-Disposition"); got != want {
		t.Fatalf("disposition = %q, want %q", got, want)
	}

	w = e.do("GET", path, nil)
	list := decode[[]archiveResp](t, w)
	if w.Code != 200 || len(list) != 1 || list[0].ID != a.ID {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := e.do("GET", fmt.Sprintf("/api/bookmarks/%d", b.ID), nil); !strings.Contains(w.Body.String(), `"archive_count":1`) {
		t.Fatal(w.Body.String())
	}

	if w := e.do("DELETE", fmt.Sprintf("/api/archives/%d", a.ID), nil); w.Code != 204 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := e.do("GET", a.ViewURL, nil); w.Code != 404 {
		t.Fatal(w.Code)
	}
	if w := e.do("DELETE", fmt.Sprintf("/api/archives/%d", a.ID), nil); w.Code != 404 {
		t.Fatal(w.Code)
	}
}

func TestUploadDefaultsAndBearer(t *testing.T) {
	e := newTestEnv(t)
	b := e.create(t, "https://example.com/a")
	ctype, body := multipartBody([][3]string{{"file", "doc.bin", "%PDF-1.4 fake"}})
	path := fmt.Sprintf("/api/bookmarks/%d/archives", b.ID)
	w := e.upload(path, ctype, strings.NewReader(body.String()), true)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	a := decode[archiveResp](t, w)
	if a.Source != "upload" || a.ContentType != "application/pdf" {
		t.Fatalf("%+v", a)
	}
	w = e.do("GET", a.ViewURL, nil)
	if got := w.Header().Get("Content-Security-Policy"); got != "default-src 'none'; object-src 'self'" {
		t.Fatalf("csp = %q", got)
	}
	w = e.do("GET", a.DownloadURL, nil)
	if !strings.HasSuffix(w.Header().Get("Content-Disposition"), `.pdf"`) {
		t.Fatal(w.Header())
	}
}

func TestUploadValidation(t *testing.T) {
	e := newTestEnv(t)
	b := e.create(t, "https://example.com/a")
	path := fmt.Sprintf("/api/bookmarks/%d/archives", b.ID)
	send := func(path string, parts [][3]string) *httptest.ResponseRecorder {
		ctype, body := multipartBody(parts)
		return e.upload(path, ctype, strings.NewReader(body.String()), false)
	}
	if w := send(path, [][3]string{{"source", "", "extension"}}); w.Code != 422 {
		t.Fatal("no file", w.Code)
	}
	if w := send(path, [][3]string{{"source", "", "bogus"}, {"file", "a.html", "x"}}); w.Code != 422 {
		t.Fatal("bad source", w.Code)
	}
	if w := send(path, [][3]string{{"created_at", "", "yesterday"}, {"file", "a.html", "x"}}); w.Code != 422 {
		t.Fatal("bad created_at", w.Code)
	}
	w := send(path, [][3]string{{"file", "a.html", "x"}, {"source", "", "extension"}})
	if w.Code != 422 || !strings.Contains(w.Body.String(), "before") {
		t.Fatal("file first", w.Code, w.Body.String())
	}
	if list := decode[[]archiveResp](t, e.do("GET", path, nil)); len(list) != 0 {
		t.Fatalf("archive kept after rejected upload: %+v", list)
	}
	if w := send("/api/bookmarks/999/archives", [][3]string{{"file", "a.html", "x"}}); w.Code != 404 {
		t.Fatal("missing bookmark", w.Code)
	}
	// JSON content type is not a valid upload.
	if w := e.do("POST", path, map[string]string{}); w.Code != 415 {
		t.Fatal("json upload", w.Code)
	}
	// Multipart is rejected on other write routes.
	ctype, body := multipartBody([][3]string{{"file", "a.html", "x"}})
	if w := e.upload("/api/bookmarks", ctype, strings.NewReader(body.String()), false); w.Code != 415 {
		t.Fatal("multipart elsewhere", w.Code)
	}
}

func TestUploadTooLarge(t *testing.T) {
	e := newTestEnv(t, func(o *envOpts) { o.maxUpload = 1024 })
	b := e.create(t, "https://example.com/a")
	ctype, body := multipartBody([][3]string{{"file", "big.html", strings.Repeat("a", 4096)}})
	w := e.upload(fmt.Sprintf("/api/bookmarks/%d/archives", b.ID), ctype, strings.NewReader(body.String()), false)
	if w.Code != 413 {
		t.Fatal(w.Code, w.Body.String())
	}
	var files []string
	_ = filepath.WalkDir(filepath.Join(e.dir, "archives"), func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			files = append(files, p)
		}
		return nil
	})
	if len(files) != 0 {
		t.Fatalf("files left: %v", files)
	}
}

func TestFileNameTraversal(t *testing.T) {
	e := newTestEnv(t)
	if err := os.WriteFile(filepath.Join(e.dir, "secret.txt"), []byte("s"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"/favicons/..%2Fbookmarks.db", "/previews/..%2Fbookmarks.db", "/favicons/.hidden", "/favicons/missing.png", "/favicons/..%2Fsecret.txt"} {
		if w := e.do("GET", p, nil); w.Code != 404 {
			t.Errorf("%s -> %d", p, w.Code)
		}
	}
}

func TestFileNameDotDotIsNotServed(t *testing.T) {
	// The mux cleans the path and redirects; either way nothing is served.
	e := newTestEnv(t)
	if w := e.do("GET", "/favicons/..", nil); w.Code == 200 {
		t.Fatal(w.Code)
	}
}

func TestFaviconAndPreviewServing(t *testing.T) {
	e := newTestEnv(t)
	for _, d := range []string{"favicons", "previews"} {
		if err := os.MkdirAll(filepath.Join(e.dir, d), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	svg := `<svg xmlns="http://www.w3.org/2000/svg" width="1" height="1"></svg>`
	png := "\x89PNG\r\n\x1a\n" + strings.Repeat("x", 20)
	_ = os.WriteFile(filepath.Join(e.dir, "favicons", "a.png"), []byte(svg), 0o600)
	_ = os.WriteFile(filepath.Join(e.dir, "previews", "p.png"), []byte(png), 0o600)
	w := e.do("GET", "/favicons/a.png", nil)
	if w.Code != 200 || w.Header().Get("Content-Type") != "image/svg+xml" || w.Body.String() != svg {
		t.Fatal(w.Code, w.Header())
	}
	if w.Header().Get("X-Content-Type-Options") != "nosniff" ||
		w.Header().Get("Content-Security-Policy") != "default-src 'none'; style-src 'unsafe-inline'; sandbox" ||
		w.Header().Get("Cache-Control") != "public, max-age=86400" {
		t.Fatal(w.Header())
	}
	w = e.do("GET", "/previews/p.png", nil)
	if w.Code != 200 || w.Header().Get("Content-Type") != "image/png" {
		t.Fatal(w.Code, w.Header())
	}
}

func archiveCount(t *testing.T, e *testEnv, bookmarkID int64) int {
	t.Helper()
	w := e.do("GET", fmt.Sprintf("/api/bookmarks/%d/archives", bookmarkID), nil)
	return len(decode[[]archiveResp](t, w))
}

func TestUploadFailureAfterFileLeavesNothing(t *testing.T) {
	e := newTestEnv(t)
	b := e.create(t, "https://example.com/a")
	path := fmt.Sprintf("/api/bookmarks/%d/archives", b.ID)
	ctype, body := multipartBody([][3]string{{"file", "a.html", "one"}, {"file", "b.html", "two"}})
	if w := e.upload(path, ctype, strings.NewReader(body.String()), false); w.Code != 422 {
		t.Fatal(w.Code, w.Body.String())
	}
	if n := archiveCount(t, e, b.ID); n != 0 {
		t.Fatalf("two files left %d archives", n)
	}
	// Truncated trailing part: file complete, then garbage without a closing boundary.
	ctype, body = multipartBody([][3]string{{"file", "a.html", "one"}})
	broken := strings.Replace(body.String(), "--XBOUNDARYX--\r\n", "--XBOUNDARYX\r\nContent-Disposition: form-data; name=\"x\"\r\n\r\npartial", 1)
	if w := e.upload(path, ctype, strings.NewReader(broken), false); w.Code != 400 {
		t.Fatal(w.Code, w.Body.String())
	}
	if n := archiveCount(t, e, b.ID); n != 0 {
		t.Fatalf("truncated upload left %d archives", n)
	}
}

func TestUploadOverBodyLimit(t *testing.T) {
	// File fits the service limit, but the whole request exceeds limit + 1 MiB
	// because of a large trailing field: MaxBytesError on a later part.
	e := newTestEnv(t, func(o *envOpts) { o.maxUpload = 1024 })
	b := e.create(t, "https://example.com/a")
	ctype, body := multipartBody([][3]string{{"file", "a.html", "small"}, {"pad", "", strings.Repeat("p", 2<<20)}})
	w := e.upload(fmt.Sprintf("/api/bookmarks/%d/archives", b.ID), ctype, strings.NewReader(body.String()), false)
	if w.Code != 413 {
		t.Fatal(w.Code, w.Body.String())
	}
	if n := archiveCount(t, e, b.ID); n != 0 {
		t.Fatalf("%d archives left", n)
	}
	// And while streaming the file part itself.
	ctype, body = multipartBody([][3]string{{"file", "a.html", strings.Repeat("a", 2<<20)}})
	w = e.upload(fmt.Sprintf("/api/bookmarks/%d/archives", b.ID), ctype, strings.NewReader(body.String()), false)
	if w.Code != 413 {
		t.Fatal(w.Code, w.Body.String())
	}
	var files []string
	_ = filepath.WalkDir(filepath.Join(e.dir, "archives"), func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			files = append(files, p)
		}
		return nil
	})
	if len(files) != 0 {
		t.Fatalf("files left: %v", files)
	}
}

func TestBookmarkImageURLsEndToEnd(t *testing.T) {
	e := newTestEnv(t)
	b := e.create(t, "https://example.com/a")
	st, err := store.Open(filepath.Join(e.dir, "bookmarks.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	bm, err := st.GetBookmark(context.Background(), b.ID)
	if err != nil {
		t.Fatal(err)
	}
	bm.FaviconFile, bm.PreviewFile = "f.png", "p.jpg"
	if err := st.UpdateBookmark(context.Background(), bm); err != nil {
		t.Fatal(err)
	}
	w := e.do("GET", fmt.Sprintf("/api/bookmarks/%d", b.ID), nil)
	for _, s := range []string{`"favicon_url":"/favicons/f.png"`, `"preview_image_url":"/previews/p.jpg"`} {
		if !strings.Contains(w.Body.String(), s) {
			t.Errorf("missing %s in %s", s, w.Body)
		}
	}
}
