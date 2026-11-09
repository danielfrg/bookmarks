package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAuthMatrix(t *testing.T) {
	cases := []struct {
		name      string
		method    string
		auth      string
		xbm       string
		origin    string
		noToken   bool
		wantCode  int
		wantTexts string
	}{
		{name: "GET no headers", method: "GET", wantCode: 200},
		{name: "POST no header", method: "POST", wantCode: 403},
		{name: "POST x-bookmarks same origin", method: "POST", xbm: "1", origin: "http://example.com", wantCode: 201},
		{name: "POST cross origin", method: "POST", xbm: "1", origin: "http://evil.example", wantCode: 403},
		{name: "POST bearer good", method: "POST", auth: "Bearer " + testToken, origin: "chrome-extension://abc", wantCode: 201},
		{name: "POST bearer scheme case-insensitive", method: "POST", auth: "bearer " + testToken, wantCode: 201},
		{name: "POST Token scheme", method: "POST", auth: "Token " + testToken, wantCode: 201},
		{name: "POST bearer bad", method: "POST", auth: "Bearer nope", wantCode: 401, wantTexts: `{"detail":"invalid API token"}`},
		{name: "GET bearer bad", method: "GET", auth: "Bearer nope", wantCode: 401},
		{name: "other scheme ignored, writes need X-Bookmarks", method: "POST", auth: "Basic xyz", wantCode: 403},
		{name: "other scheme ignored, same origin write", method: "POST", auth: "Basic xyz", xbm: "1", origin: "http://example.com", wantCode: 201},
		{name: "other scheme ignored, GET", method: "GET", auth: "Basic abc", wantCode: 200},
		{name: "bearer without configured token", method: "POST", auth: "Bearer " + testToken, noToken: true, wantCode: 401},
		{name: "bearer empty token without configured token", method: "GET", auth: "Bearer ", noToken: true, wantCode: 401},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newTestEnv(t, func(o *envOpts) { o.noToken = c.noToken })
			path := "/_/api/bookmarks"
			var body *strings.Reader
			if c.method == "POST" {
				body = strings.NewReader(`{"url":"https://example.com/a"}`)
			} else {
				body = strings.NewReader("")
			}
			r := httptest.NewRequest(c.method, path+"?capture=none", body)
			if c.method == "POST" {
				r.Header.Set("Content-Type", "application/json")
			}
			if c.auth != "" {
				r.Header.Set("Authorization", c.auth)
			}
			if c.xbm != "" {
				r.Header.Set("X-Bookmarks", c.xbm)
			}
			if c.origin != "" {
				r.Header.Set("Origin", c.origin)
			}
			w := httptest.NewRecorder()
			e.h.ServeHTTP(w, r)
			if w.Code != c.wantCode {
				t.Fatalf("status = %d, want %d: %s", w.Code, c.wantCode, w.Body)
			}
			if c.wantTexts != "" {
				if got := strings.TrimSpace(w.Body.String()); got != c.wantTexts {
					t.Fatalf("body = %q", got)
				}
				if ct := w.Header().Get("Content-Type"); ct != "application/json" {
					t.Fatalf("content-type = %q", ct)
				}
			}
		})
	}
}

func TestWriteRequiresJSONBody(t *testing.T) {
	e := newTestEnv(t)
	r := httptest.NewRequest("POST", "/_/api/bookmarks", strings.NewReader("url=x"))
	r.Header.Set("X-Bookmarks", "1")
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, r)
	if w.Code != http.StatusUnsupportedMediaType {
		t.Fatal(w.Code)
	}
}

func TestJSONBodyLimit(t *testing.T) {
	e := newTestEnv(t)
	big := strings.Repeat("a", 2<<20)
	r := httptest.NewRequest("POST", "/_/api/bookmarks", strings.NewReader(`{"url":"https://example.com","notes":"`+big+`"}`))
	r.Header.Set("X-Bookmarks", "1")
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, r)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatal(w.Code, w.Body.String())
	}
}
