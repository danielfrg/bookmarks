package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
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
	if w := get(ok, "/_/healthz"); w.Code != 200 || w.Body.String() != "ok\n" {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := get(ok, "/_/readyz"); w.Code != 200 {
		t.Fatal(w.Code)
	}
	bad := NewHandler(fstest.MapFS{}, Deps{Ping: func(context.Context) error { return errors.New("down") }})
	if w := get(bad, "/_/readyz"); w.Code != 503 {
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
	if w := get(h, "/_/api/openapi.json"); w.Code != 200 {
		t.Fatal(w.Code)
	}
}
