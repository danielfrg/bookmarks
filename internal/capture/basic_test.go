package capture

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/danielfrg/bookmarks/internal/fetch"
)

// 1x1 transparent PNG.
var pngBytes = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89\x00\x00\x00\x00IEND\xaeB`\x82")

func newTestBasic(t *testing.T, h http.Handler) (*Basic, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	b := NewBasic(fetch.NewClient(fetch.Options{AllowPrivate: true})).(*Basic)
	b.now = func() time.Time { return time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC) }
	return b, srv
}

const page = `<!doctype html><html><head><meta charset="iso-8859-1"><title>T</title>
<link rel="stylesheet" href="/s.css" media="print">
<link rel="preload" href="/p.js" as="script">
<link rel="icon" href="/fav.ico">
<script>alert(1)</script></head>
<body onload="x()"><noscript>no</noscript><iframe src="/f"></iframe>
<img src="/i.png" srcset="/i2.png 2x"><img src="/missing.png">
<a href="/about" onclick="y()">about</a><a href="javascript:evil()">j</a></body></html>`

func TestBasicCaptureInlines(t *testing.T) {
	b, srv := newTestBasic(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/page":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write([]byte(page))
		case "/s.css":
			w.Header().Set("Content-Type", "text/css")
			w.Write([]byte("body{background:url(bg.png)} p{color:red}"))
		case "/i.png":
			w.Header().Set("Content-Type", "image/png")
			w.Write(pngBytes)
		default:
			http.NotFound(w, r)
		}
	}))
	res, err := b.Capture(context.Background(), srv.URL+"/page")
	if err != nil {
		t.Fatal(err)
	}
	if res.ContentType != "text/html; charset=utf-8" {
		t.Errorf("content type %q", res.ContentType)
	}
	out := string(res.Body)
	for _, bad := range []string{"<script", "alert(1)", "<noscript", "<iframe", "onload", "onclick", "javascript:", "preload", "iso-8859-1", "srcset", "<link rel=\"stylesheet\""} {
		if strings.Contains(out, bad) {
			t.Errorf("output contains %q:\n%s", bad, out)
		}
	}
	for _, want := range []string{
		"data:image/png;base64,",
		"p{color:red}",
		`<style media="print">`,
		"url(" + srv.URL + "/bg.png)",
		`href="` + srv.URL + `/about"`,
		`href="` + srv.URL + `/fav.ico"`,
		`<meta charset="utf-8"/>`,
		"<!-- bookmarks basic capture of " + srv.URL + "/page at 2026-11-09T12:00:00Z -->",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if i, j := strings.Index(out, `<meta charset="utf-8"/>`), strings.Index(out, "<!-- bookmarks"); i < 0 || j < i {
		t.Errorf("comment should follow meta charset")
	}
}

func TestBasicCapturePDF(t *testing.T) {
	b, srv := newTestBasic(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		w.Write([]byte("%PDF-1.4 hi"))
	}))
	res, err := b.Capture(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if res.ContentType != "application/pdf" || string(res.Body) != "%PDF-1.4 hi" {
		t.Errorf("got %q %q", res.ContentType, res.Body)
	}
}

func TestBasicCaptureResourceFailureKeepsURL(t *testing.T) {
	b, srv := newTestBasic(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/page" {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<html><body><img src="/gone.png"></body></html>`))
			return
		}
		http.NotFound(w, r)
	}))
	res, err := b.Capture(context.Background(), srv.URL+"/page")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(res.Body), `src="`+srv.URL+`/gone.png"`) {
		t.Errorf("absolute URL not kept:\n%s", res.Body)
	}
}

func TestBasicCaptureErrors(t *testing.T) {
	b, srv := newTestBasic(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/404":
			http.NotFound(w, r)
		case "/json":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("{}"))
		}
	}))
	var he *HTTPError
	if _, err := b.Capture(context.Background(), srv.URL+"/404"); !errors.As(err, &he) || he.Status != 404 {
		t.Errorf("want HTTPError 404, got %v", err)
	}
	if _, err := b.Capture(context.Background(), srv.URL+"/json"); !errors.Is(err, ErrUnsupported) {
		t.Errorf("want ErrUnsupported, got %v", err)
	}
}

func TestBasicCaptureCharsetAndBase(t *testing.T) {
	b, srv := newTestBasic(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/page" {
			w.Header().Set("Content-Type", "text/html; charset=iso-8859-1")
			w.Write([]byte("<html><head><base href=\"/sub/\"></head><body>caf\xe9 <a href=\"x\">x</a></body></html>"))
		}
	}))
	res, err := b.Capture(context.Background(), srv.URL+"/page")
	if err != nil {
		t.Fatal(err)
	}
	out := string(res.Body)
	if !strings.Contains(out, "café") {
		t.Errorf("not decoded to utf-8: %s", out)
	}
	if strings.Contains(out, "<base") || !strings.Contains(out, `href="`+srv.URL+`/sub/x"`) {
		t.Errorf("base not honored:\n%s", out)
	}
}

func TestBasicCaptureNoHeadAndRedirect(t *testing.T) {
	b, srv := newTestBasic(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/old":
			http.Redirect(w, r, "/dir/new", http.StatusFound)
		case "/dir/new":
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<a href="rel">r</a>`))
		}
	}))
	res, err := b.Capture(context.Background(), srv.URL+"/old")
	if err != nil {
		t.Fatal(err)
	}
	out := string(res.Body)
	if !strings.Contains(out, `<head><meta charset="utf-8"/>`) || !strings.Contains(out, `href="`+srv.URL+`/dir/rel"`) {
		t.Errorf("unexpected output:\n%s", out)
	}
}

func TestBasicCaptureResourceLimit(t *testing.T) {
	var hits int
	b, srv := newTestBasic(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/page" {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(strings.Repeat(`<img src="/i.png">`, 105)))
			return
		}
		hits++
		w.Header().Set("Content-Type", "image/png")
		w.Write(pngBytes)
	}))
	if _, err := b.Capture(context.Background(), srv.URL+"/page"); err != nil {
		t.Fatal(err)
	}
	if hits != 100 {
		t.Errorf("resource fetches = %d, want 100", hits)
	}
}

func TestBasicCaptureCanceled(t *testing.T) {
	b, srv := newTestBasic(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := b.Capture(ctx, srv.URL); err == nil {
		t.Error("want error")
	}
}
