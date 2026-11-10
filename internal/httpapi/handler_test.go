package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func get(h http.Handler, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	return w
}

func TestHealth(t *testing.T) {
	ok := NewHandler(fstest.MapFS{}, Deps{Ping: func(context.Context) error { return nil }})
	if w := get(ok, "/healthz"); w.Code != 200 || w.Body.String() != "ok\n" {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := get(ok, "/readyz"); w.Code != 200 {
		t.Fatal(w.Code)
	}
	bad := NewHandler(fstest.MapFS{}, Deps{Ping: func(context.Context) error { return errors.New("down") }})
	if w := get(bad, "/readyz"); w.Code != 503 {
		t.Fatal(w.Code)
	}
}

func TestIndexWithoutFrontend(t *testing.T) {
	h := NewHandler(fstest.MapFS{}, Deps{Ping: func(context.Context) error { return nil }})
	if w := get(h, "/"); w.Code != 503 {
		t.Fatal(w.Code)
	}
	h = NewHandler(fstest.MapFS{"index.html": {Data: []byte("hi")}}, Deps{Ping: func(context.Context) error { return nil }})
	if w := get(h, "/"); w.Code != 200 || w.Body.String() != "hi" {
		t.Fatal(w.Code)
	}
}

func TestOpenAPIServed(t *testing.T) {
	h := NewHandler(fstest.MapFS{}, Deps{Ping: func(context.Context) error { return nil }})
	if w := get(h, "/api/openapi.json"); w.Code != 200 {
		t.Fatal(w.Code)
	}
}

func TestMCPMountedOutsideAPIGuard(t *testing.T) {
	e := newTestEnv(t)
	r := httptest.NewRequest("POST", "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept", "application/json, text/event-stream")
	r.Host = "localhost"
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

func TestPagePathsServeIndex(t *testing.T) {
	h := NewHandler(fstest.MapFS{"index.html": {Data: []byte("hi")}}, Deps{Ping: func(context.Context) error { return nil }})
	for _, p := range []string{"/tags", "/saved", "/settings", "/settings/general", "/settings/api", "/settings/import-export"} {
		if w := get(h, p); w.Code != 200 || w.Body.String() != "hi" {
			t.Errorf("%s: %d", p, w.Code)
		}
	}
	for _, p := range []string{"/nope", "/tags/x", "/settings/bogus", "/settings/api/x", "/saved/1"} {
		if w := get(h, p); w.Code != 404 {
			t.Errorf("%s: %d, want 404", p, w.Code)
		}
	}
}
