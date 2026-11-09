package fetch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseMetadata(t *testing.T) {
	cases := []struct {
		name, html string
		want       Metadata
	}{
		{"title and description",
			`<html><head><title>  Hello
 World </title><meta name="description" content="Desc"></head><body></body></html>`,
			Metadata{Title: "Hello World", Description: "Desc"}},
		{"og fallback and relative image",
			`<head><title>T</title><meta property="og:description" content="OG"><meta property="og:image" content="/img/a.png"></head>`,
			Metadata{Title: "T", Description: "OG", PreviewImageURL: "https://example.com/img/a.png"}},
		{"description wins over og",
			`<head><meta property="og:description" content="OG"><meta name="description" content="D"></head>`,
			Metadata{Description: "D"}},
		{"stops at head end",
			`<head><title>A</title></head><body><title>B</title><meta name="description" content="late"></body>`,
			Metadata{Title: "A"}},
		{"stops at body start without head end",
			`<head><title>A</title><body><meta property="og:image" content="x.png">`,
			Metadata{Title: "A"}},
		{"charset in meta",
			"<head><meta charset=\"windows-1252\"><title>caf\xe9</title></head>",
			Metadata{Title: "café"}},
		{"http-equiv charset",
			"<head><meta http-equiv=\"Content-Type\" content=\"text/html; charset=windows-1252\"><title>caf\xe9</title></head>",
			Metadata{Title: "café"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseMetadata([]byte(tc.html), "https://example.com/page")
			if got != tc.want {
				t.Fatalf("got %+v want %+v", got, tc.want)
			}
		})
	}
}

func TestMetadataContentTypeCharset(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=iso-8859-1")
		w.Write([]byte("<head><title>caf\xe9</title></head>"))
	}))
	defer srv.Close()
	c := NewClient(Options{AllowPrivate: true})
	m, err := c.Metadata(context.Background(), srv.URL)
	if err != nil || m.Title != "café" {
		t.Fatalf("m=%+v err=%v", m, err)
	}
}

func TestMetadataNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", 500)
	}))
	defer srv.Close()
	c := NewClient(Options{AllowPrivate: true})
	if _, err := c.Metadata(context.Background(), srv.URL); err == nil {
		t.Fatal("expected error")
	}
}

func TestMetadataRelativeImageUsesFinalURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "/dir/final", http.StatusFound)
			return
		}
		w.Write([]byte(`<head><meta property="og:image" content="i.png"></head>`))
	}))
	defer srv.Close()
	c := NewClient(Options{AllowPrivate: true})
	m, err := c.Metadata(context.Background(), srv.URL+"/start")
	if err != nil || !strings.HasSuffix(m.PreviewImageURL, "/dir/i.png") {
		t.Fatalf("m=%+v err=%v", m, err)
	}
}

func TestMetadataHeaderAndMetaCharsetDecodedOnce(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=windows-1252")
		w.Write([]byte("<head><meta charset=\"windows-1252\"><title>caf\xe9</title></head>"))
	}))
	defer srv.Close()
	c := NewClient(Options{AllowPrivate: true})
	m, err := c.Metadata(context.Background(), srv.URL)
	if err != nil || m.Title != "café" {
		t.Fatalf("m=%+v err=%v", m, err)
	}
}
