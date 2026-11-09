package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/danielfrg/bookmarks/internal/bookmarks"
	"github.com/danielfrg/bookmarks/internal/capture"
	"github.com/danielfrg/bookmarks/internal/fetch"
	"github.com/danielfrg/bookmarks/internal/jobs"
)

func runJob(t *testing.T, e *env, kind jobs.Kind, id int64) error {
	t.Helper()
	return runJobCtx(context.Background(), t, e, kind, id)
}

func runJobCtx(ctx context.Context, t *testing.T, e *env, kind jobs.Kind, id int64) error {
	t.Helper()
	h, ok := e.svc.JobHandlers()[kind]
	if !ok {
		t.Fatalf("no handler for %s", kind)
	}
	return h(ctx, jobs.Job{ID: 1, BookmarkID: id, Kind: kind, Attempts: 1})
}

func addFileArchive(t *testing.T, e *env, id int64, ct, path string) bookmarks.Archive {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	a, err := e.svc.AddArchive(context.Background(), id, ArchiveInput{Source: bookmarks.SourceUpload, ContentType: ct, Body: bytes.NewReader(data)})
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestExtractJobRecordsFailure(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	b, _, _ := e.svc.Save(ctx, SaveInput{URL: "https://a.example"})
	addFileArchive(t, e, b.ID, "text/html", "../reader/testdata/nav-only.html")

	if err := runJob(t, e, jobs.KindExtract, b.ID); err != nil {
		t.Fatalf("handler err = %v, want nil", err)
	}
	r, _ := e.st.GetReaderText(ctx, b.ID)
	if r.Status != bookmarks.ReaderFailed || r.Error == "" {
		t.Fatalf("reader = %+v", r)
	}
}

func TestExtractJobSuccessAndNone(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	b, _, _ := e.svc.Save(ctx, SaveInput{URL: "https://a.example"})

	if err := runJob(t, e, jobs.KindExtract, b.ID); err != nil {
		t.Fatal(err)
	}
	r, _ := e.st.GetReaderText(ctx, b.ID)
	if r.Status != bookmarks.ReaderNone {
		t.Fatalf("status = %q, want none", r.Status)
	}

	a := addFileArchive(t, e, b.ID, "text/html", "../reader/testdata/article.html")
	if err := runJob(t, e, jobs.KindExtract, b.ID); err != nil {
		t.Fatal(err)
	}
	r, _ = e.st.GetReaderText(ctx, b.ID)
	if r.Status != bookmarks.ReaderOK || r.ArchiveID != a.ID || !strings.Contains(r.Markdown, "searchable memory") || r.WordCount == 0 {
		t.Fatalf("reader = %+v", r)
	}
	if !r.ExtractedAt.Equal(fixedNow) {
		t.Fatalf("extracted at %v", r.ExtractedAt)
	}

	// A deleted bookmark completes the job.
	if err := runJob(t, e, jobs.KindExtract, 999); err != nil {
		t.Fatalf("missing bookmark err = %v", err)
	}
}

func TestExtractJobPropagatesCancel(t *testing.T) {
	e := newEnv(t)
	b, _, _ := e.svc.Save(context.Background(), SaveInput{URL: "https://a.example"})
	addFileArchive(t, e, b.ID, "text/html", "../reader/testdata/article.html")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := runJobCtx(ctx, t, e, jobs.KindExtract, b.ID)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	r, _ := e.st.GetReaderText(context.Background(), b.ID)
	if r.Status == bookmarks.ReaderFailed {
		t.Fatal("cancellation recorded as failure")
	}
}

func TestCaptureJobAddsServerArchive(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	b, _, _ := e.svc.Save(ctx, SaveInput{URL: "https://a.example"})
	e.q.reset()
	e.cap.res = capture.Result{ContentType: "text/html; charset=utf-8", Body: []byte("<html>captured</html>")}

	if err := runJob(t, e, jobs.KindCapture, b.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := e.svc.Get(ctx, b.ID)
	if !got.BasicCaptureOnly || got.ArchiveCount != 1 {
		t.Fatalf("bookmark = %+v", got)
	}
	list, _ := e.st.ListArchives(ctx, b.ID)
	if list[0].Source != bookmarks.SourceServer || list[0].DisplayName != "Basic capture" || list[0].ContentType != "text/html" {
		t.Fatalf("archive = %+v", list[0])
	}
	if k := e.q.kinds(b.ID); len(k) != 1 || k[0] != jobs.KindExtract {
		t.Fatalf("jobs = %v", k)
	}
}

func TestCaptureJobErrors(t *testing.T) {
	e := newEnv(t)
	b, _, _ := e.svc.Save(context.Background(), SaveInput{URL: "https://a.example"})
	for _, tc := range []struct {
		err       error
		permanent bool
	}{
		{&capture.HTTPError{Status: 404}, true},
		{&capture.HTTPError{Status: 503}, false},
		{fmt.Errorf("dial: %w", fetch.ErrBlockedAddress), true},
		{capture.ErrUnsupported, true},
		{capture.ErrTooLarge, true},
		{errors.New("timeout"), false},
	} {
		e.cap.res = capture.Result{}
		e.cap.err = tc.err
		err := runJob(t, e, jobs.KindCapture, b.ID)
		if err == nil || errors.Is(err, jobs.ErrPermanent) != tc.permanent {
			t.Fatalf("capture err %v → %v, permanent want %v", tc.err, err, tc.permanent)
		}
	}
}

func TestCaptureJobTooLargeIsPermanent(t *testing.T) {
	e := newEnv(t)
	b, _, _ := e.svc.Save(context.Background(), SaveInput{URL: "https://a.example"})
	e.cap.res = capture.Result{ContentType: "text/html", Body: bytes.Repeat([]byte("x"), 1<<20+1)}
	if err := runJob(t, e, jobs.KindCapture, b.ID); !errors.Is(err, jobs.ErrPermanent) {
		t.Fatalf("err = %v, want permanent", err)
	}
}

func TestMetadataJobReplacesPreview(t *testing.T) {
	var srvURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/page":
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprintf(w, `<html><head><meta property="og:image" content="%s/new.png"></head></html>`, srvURL)
		case "/new.png":
			w.Header().Set("Content-Type", "image/png")
			w.Write([]byte("\x89PNG"))
		}
	}))
	defer srv.Close()
	srvURL = srv.URL
	e := newEnv(t)
	ctx := context.Background()
	previews := filepath.Join(e.dir, "previews")
	os.MkdirAll(previews, 0o755)
	for _, f := range []string{"old.png", "shared.png"} {
		os.WriteFile(filepath.Join(previews, f), []byte("png"), 0o644)
	}
	mk := func(u, preview string) bookmarks.Bookmark {
		b, _, _ := e.svc.Save(ctx, SaveInput{URL: u, Title: ptr("t"), Description: ptr("d")})
		b.PreviewFile = preview
		if err := e.st.UpdateBookmark(ctx, b); err != nil {
			t.Fatal(err)
		}
		return b
	}
	solo := mk(srv.URL+"/page", "old.png")
	shared := mk(srv.URL+"/page?x=1", "shared.png")
	mk("https://other.example", "shared.png")

	for _, b := range []bookmarks.Bookmark{solo, shared} {
		if err := runJob(t, e, jobs.KindMetadata, b.ID); err != nil {
			t.Fatal(err)
		}
	}
	got, _ := e.svc.Get(ctx, solo.ID)
	if got.PreviewFile == "old.png" || got.PreviewFile == "" {
		t.Fatalf("preview = %q", got.PreviewFile)
	}
	if _, err := os.Stat(filepath.Join(previews, "old.png")); !os.IsNotExist(err) {
		t.Fatalf("old preview not removed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(previews, "shared.png")); err != nil {
		t.Fatalf("shared preview removed: %v", err)
	}
}

func TestMetadataJob(t *testing.T) {
	var srvURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/page":
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprintf(w, `<html><head><title>Fetched</title><meta name="description" content="Fetched desc">
<meta property="og:image" content="%s/img.png"></head></html>`, srvURL)
		case "/img.png", "/favicon":
			w.Header().Set("Content-Type", "image/png")
			w.Write([]byte("\x89PNG"))
		case "/gone":
			w.WriteHeader(500)
		}
	}))
	defer srv.Close()
	srvURL = srv.URL

	e := newEnv(t)
	e.svc.o.FaviconProvider = srv.URL + "/favicon?u={url}"
	ctx := context.Background()
	b, _, _ := e.svc.Save(ctx, SaveInput{URL: srv.URL + "/page", Title: ptr("Mine")})
	if err := runJob(t, e, jobs.KindMetadata, b.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := e.svc.Get(ctx, b.ID)
	if got.Title != "Mine" || got.Description != "Fetched desc" {
		t.Fatalf("bookmark = %+v", got)
	}
	if got.FaviconFile == "" || got.PreviewFile == "" {
		t.Fatalf("images = %q %q", got.FaviconFile, got.PreviewFile)
	}
	if _, err := os.Stat(filepath.Join(e.dir, "favicons", got.FaviconFile)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(e.dir, "previews", got.PreviewFile)); err != nil {
		t.Fatal(err)
	}
	if !got.DateModified.Equal(b.DateModified) {
		t.Fatal("metadata job changed DateModified")
	}

	b2, _, _ := e.svc.Save(ctx, SaveInput{URL: srv.URL + "/gone"})
	if err := runJob(t, e, jobs.KindMetadata, b2.ID); err == nil || errors.Is(err, jobs.ErrPermanent) {
		t.Fatalf("err = %v, want retryable", err)
	}

	// Blocked addresses are permanent failures.
	e.svc.fc = fetch.NewClient(fetch.Options{})
	if err := runJob(t, e, jobs.KindMetadata, b2.ID); !errors.Is(err, jobs.ErrPermanent) {
		t.Fatalf("err = %v, want permanent", err)
	}
}

func TestWaybackJob(t *testing.T) {
	var mode atomic.Int32 // 0 redirect, 1 plain 200, 2 server error
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case mode.Load() == 2:
			w.WriteHeader(502)
		case strings.HasPrefix(r.URL.Path, "/save/") && mode.Load() == 0:
			w.Header().Set("Location", "/web/20261109120000/https://a.example/")
			w.WriteHeader(http.StatusFound)
		case strings.HasPrefix(r.URL.Path, "/save/"):
			w.WriteHeader(200)
		case strings.HasPrefix(r.URL.Path, "/web/"):
			w.WriteHeader(200)
		}
	}))
	defer srv.Close()

	e := newEnv(t)
	e.svc.waybackBase = srv.URL
	ctx := context.Background()
	b, _, _ := e.svc.Save(ctx, SaveInput{URL: "https://a.example/"})
	if err := runJob(t, e, jobs.KindWayback, b.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := e.svc.Get(ctx, b.ID)
	if got.WebArchiveURL != srv.URL+"/web/20261109120000/https://a.example/" {
		t.Fatalf("web archive url = %q", got.WebArchiveURL)
	}

	mode.Store(1)
	added := fixedNow.AddDate(-1, 0, 0)
	b2, _, _ := e.svc.Save(ctx, SaveInput{URL: "https://b.example/x", DateAdded: &added})
	if err := runJob(t, e, jobs.KindWayback, b2.ID); err != nil {
		t.Fatal(err)
	}
	got, _ = e.svc.Get(ctx, b2.ID)
	if got.WebArchiveURL != srv.URL+"/web/20251109120000/https://b.example/x" {
		t.Fatalf("fallback url = %q", got.WebArchiveURL)
	}

	mode.Store(2)
	if err := runJob(t, e, jobs.KindWayback, b2.ID); err == nil || errors.Is(err, jobs.ErrPermanent) {
		t.Fatalf("err = %v, want retryable", err)
	}
	srv.Close()
	if err := runJob(t, e, jobs.KindWayback, b2.ID); err == nil || errors.Is(err, jobs.ErrPermanent) {
		t.Fatalf("err = %v, want retryable", err)
	}
}
