package fetch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func serve(ct string, body []byte) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ct != "" {
			w.Header().Set("Content-Type", ct)
		}
		w.Write(body)
	}))
}

func entries(t *testing.T, dir string) []string {
	es, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range es {
		out = append(out, e.Name())
	}
	return out
}

func TestDownloadPreviewRejectsType(t *testing.T) {
	srv := serve("text/html", []byte("<html></html>"))
	defer srv.Close()
	dir := t.TempDir()
	c := NewClient(Options{AllowPrivate: true})
	if name, err := c.DownloadPreview(context.Background(), srv.URL+"/a", dir); err == nil {
		t.Fatalf("expected error, got %q", name)
	}
	if got := entries(t, dir); len(got) != 0 {
		t.Fatalf("files written: %v", got)
	}
}

func TestDownloadPreviewOK(t *testing.T) {
	for ct, ext := range map[string]string{
		"image/jpeg": ".jpg", "image/png": ".png", "image/gif": ".gif",
		"image/svg+xml": ".svg", "image/webp": ".webp", "image/png; charset=binary": ".png",
	} {
		srv := serve(ct, []byte("data"))
		dir := t.TempDir()
		c := NewClient(Options{AllowPrivate: true})
		u := srv.URL + "/p"
		name, err := c.DownloadPreview(context.Background(), u, dir)
		srv.Close()
		if err != nil {
			t.Fatalf("%s: %v", ct, err)
		}
		sum := sha256.Sum256([]byte(u))
		if want := hex.EncodeToString(sum[:]) + ext; name != want {
			t.Fatalf("%s: name %q want %q", ct, name, want)
		}
		if b, _ := os.ReadFile(filepath.Join(dir, name)); string(b) != "data" {
			t.Fatalf("content %q", b)
		}
		if len(entries(t, dir)) != 1 {
			t.Fatalf("stray files: %v", entries(t, dir))
		}
	}
}

func TestDownloadPreviewTooLarge(t *testing.T) {
	srv := serve("image/png", []byte(strings.Repeat("a", maxImageBytes+1)))
	defer srv.Close()
	dir := t.TempDir()
	c := NewClient(Options{AllowPrivate: true})
	if _, err := c.DownloadPreview(context.Background(), srv.URL, dir); err == nil {
		t.Fatal("expected error")
	}
	if got := entries(t, dir); len(got) != 0 {
		t.Fatalf("files written: %v", got)
	}
}

func TestDownloadPreviewNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	dir := t.TempDir()
	c := NewClient(Options{AllowPrivate: true})
	if _, err := c.DownloadPreview(context.Background(), srv.URL, dir); err == nil {
		t.Fatal("expected error")
	}
}

func TestDownloadFavicon(t *testing.T) {
	var gotURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURL = r.URL.Query().Get("url")
		w.Header().Set("Content-Type", "image/x-icon")
		w.Write([]byte("icon"))
	}))
	defer srv.Close()
	dir := t.TempDir()
	c := NewClient(Options{AllowPrivate: true})
	name, err := c.DownloadFavicon(context.Background(), srv.URL+"/f?url={url}&size=32", "https://Sub.Example.com:8443/some/path?q=1", dir)
	if err != nil {
		t.Fatal(err)
	}
	if gotURL != "https://sub.example.com:8443" {
		t.Fatalf("provider got url=%q", gotURL)
	}
	if name != "sub.example.com.png" {
		t.Fatalf("name = %q", name)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, name)); string(b) != "icon" {
		t.Fatalf("content %q", b)
	}
	if len(entries(t, dir)) != 1 {
		t.Fatalf("stray files: %v", entries(t, dir))
	}
}

func TestDownloadFaviconRejectsNonImage(t *testing.T) {
	srv := serve("text/html", []byte("x"))
	defer srv.Close()
	dir := t.TempDir()
	c := NewClient(Options{AllowPrivate: true})
	if _, err := c.DownloadFavicon(context.Background(), srv.URL+"/?u={url}", "https://example.com", dir); err == nil {
		t.Fatal("expected error")
	}
	if got := entries(t, dir); len(got) != 0 {
		t.Fatalf("files written: %v", got)
	}
}
