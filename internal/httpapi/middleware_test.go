package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAuthMatrix(t *testing.T) {
	// "{token}" in auth is replaced by a valid API token, "{deleted}" by a
	// token that was created and then deleted.
	cases := []struct {
		name      string
		method    string
		auth      string
		xbm       string
		origin    string
		wantCode  int
		wantTexts string
	}{
		{name: "GET no headers", method: "GET", wantCode: 200},
		{name: "POST no header", method: "POST", wantCode: 403},
		{name: "POST x-bookmarks same origin", method: "POST", xbm: "1", origin: "http://example.com", wantCode: 201},
		{name: "POST cross origin", method: "POST", xbm: "1", origin: "http://evil.example", wantCode: 403},
		{name: "POST bearer good", method: "POST", auth: "Bearer {token}", origin: "chrome-extension://abc", wantCode: 201},
		{name: "POST bearer scheme case-insensitive", method: "POST", auth: "bearer {token}", wantCode: 201},
		{name: "POST Token scheme", method: "POST", auth: "Token {token}", wantCode: 201},
		{name: "POST bearer bad", method: "POST", auth: "Bearer nope", wantCode: 401, wantTexts: `{"detail":"invalid API token"}`},
		{name: "POST bearer deleted", method: "POST", auth: "Bearer {deleted}", wantCode: 401, wantTexts: `{"detail":"invalid API token"}`},
		{name: "POST bearer bad with browser headers", method: "POST", auth: "Bearer nope", xbm: "1", origin: "http://example.com", wantCode: 401},
		{name: "GET bearer bad", method: "GET", auth: "Bearer nope", wantCode: 401},
		{name: "GET bearer empty", method: "GET", auth: "Bearer ", wantCode: 401},
		{name: "other scheme ignored, writes need X-Bookmarks", method: "POST", auth: "Basic xyz", wantCode: 403},
		{name: "other scheme ignored, same origin write", method: "POST", auth: "Basic xyz", xbm: "1", origin: "http://example.com", wantCode: 201},
		{name: "other scheme ignored, GET", method: "GET", auth: "Basic abc", wantCode: 200},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newTestEnv(t)
			info, deleted, err := e.svc.CreateToken(context.Background(), "gone")
			if err != nil {
				t.Fatal(err)
			}
			if err := e.svc.DeleteToken(context.Background(), info.ID); err != nil {
				t.Fatal(err)
			}
			path := "/api/bookmarks"
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
				r.Header.Set("Authorization", strings.NewReplacer("{token}", e.token, "{deleted}", deleted).Replace(c.auth))
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

func TestTokenRoutesRejectBearers(t *testing.T) {
	e := newTestEnv(t)
	for _, c := range []struct{ method, path, auth string }{
		{"GET", "/api/tokens", "Bearer {token}"},
		{"POST", "/api/tokens", "Bearer {token}"},
		{"DELETE", "/api/tokens/1", "Bearer {token}"},
		{"GET", "/api/tokens", "Token {token}"},
		{"POST", "/api/tokens", "Bearer nope"},
	} {
		var body *strings.Reader
		if c.method == "POST" {
			body = strings.NewReader(`{"name":"x"}`)
		} else {
			body = strings.NewReader("")
		}
		r := httptest.NewRequest(c.method, c.path, body)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Bookmarks", "1")
		r.Header.Set("Authorization", strings.ReplaceAll(c.auth, "{token}", e.token))
		w := httptest.NewRecorder()
		e.h.ServeHTTP(w, r)
		if w.Code != http.StatusForbidden || strings.TrimSpace(w.Body.String()) != `{"detail":"API tokens are managed in the web UI"}` {
			t.Errorf("%s %s %q: %d %s", c.method, c.path, c.auth, w.Code, w.Body)
		}
	}
	list := decode[[]tokenResp](t, e.do("GET", "/api/tokens", nil))
	if len(list) != 1 {
		t.Fatalf("a bearer request created a token: %+v", list)
	}
}

func TestAuthVerificationErrorIs503(t *testing.T) {
	e := newTestEnv(t)
	e.st.Close()
	r := httptest.NewRequest("GET", "/api/bookmarks", nil)
	r.Header.Set("Authorization", "Bearer "+e.token)
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, r)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d: %s", w.Code, w.Body)
	}
}

func TestWriteRequiresJSONBody(t *testing.T) {
	e := newTestEnv(t)
	r := httptest.NewRequest("POST", "/api/bookmarks", strings.NewReader("url=x"))
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
	r := httptest.NewRequest("POST", "/api/bookmarks", strings.NewReader(`{"url":"https://example.com","notes":"`+big+`"}`))
	r.Header.Set("X-Bookmarks", "1")
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, r)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatal(w.Code, w.Body.String())
	}
}
