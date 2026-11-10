package httpapi

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
)

type tokenResp struct {
	ID         int64   `json:"id"`
	Name       string  `json:"name"`
	Prefix     string  `json:"prefix"`
	CreatedAt  string  `json:"created_at"`
	LastUsedAt *string `json:"last_used_at"`
	Token      string  `json:"token"`
}

func TestTokensCreateListDelete(t *testing.T) {
	e := newTestEnv(t)
	w := e.do("POST", "/api/tokens", map[string]any{"name": " CLI "})
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	created := decode[tokenResp](t, w)
	if created.ID == 0 || created.Name != "CLI" || !strings.HasPrefix(created.Token, "bk_") ||
		created.Prefix != created.Token[3:11] || created.CreatedAt == "" || created.LastUsedAt != nil {
		t.Fatalf("created %+v", created)
	}
	if !strings.Contains(w.Body.String(), `"last_used_at":null`) {
		t.Fatalf("last_used_at not null: %s", w.Body)
	}

	// The new token works as a bearer, which records its use.
	r := httptest.NewRequest("GET", "/api/bookmarks", nil)
	r.Header.Set("Authorization", "Bearer "+created.Token)
	bw := httptest.NewRecorder()
	e.h.ServeHTTP(bw, r)
	if bw.Code != 200 {
		t.Fatal(bw.Code, bw.Body.String())
	}
	lw := e.do("GET", "/api/tokens", nil)
	if lw.Code != 200 {
		t.Fatal(lw.Code, lw.Body.String())
	}
	if strings.Contains(lw.Body.String(), created.Token) || strings.Contains(lw.Body.String(), created.Token[3:]) ||
		strings.Contains(lw.Body.String(), `"token"`) || strings.Contains(lw.Body.String(), "hash") {
		t.Fatalf("list leaks the secret: %s", lw.Body)
	}
	list := decode[[]tokenResp](t, lw)
	if len(list) != 2 || list[0].ID != created.ID || list[0].LastUsedAt == nil || list[1].Name != "test" {
		t.Fatalf("list %+v", list)
	}

	if w := e.do("DELETE", fmt.Sprintf("/api/tokens/%d", created.ID), nil); w.Code != 204 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := e.do("DELETE", fmt.Sprintf("/api/tokens/%d", created.ID), nil); w.Code != 404 {
		t.Fatal(w.Code, w.Body.String())
	}
	r = httptest.NewRequest("GET", "/api/bookmarks", nil)
	r.Header.Set("Authorization", "Bearer "+created.Token)
	dw := httptest.NewRecorder()
	e.h.ServeHTTP(dw, r)
	if dw.Code != 401 {
		t.Fatalf("deleted token: %d", dw.Code)
	}
}

func TestTokensCreateValidation(t *testing.T) {
	e := newTestEnv(t)
	for _, body := range []map[string]any{{"name": ""}, {"name": "  "}, {"name": strings.Repeat("x", 101)}, {}} {
		if w := e.do("POST", "/api/tokens", body); w.Code != 422 {
			t.Errorf("%v: %d %s", body, w.Code, w.Body)
		}
	}
	// Writes follow the normal browser rules.
	r := httptest.NewRequest("POST", "/api/tokens", strings.NewReader(`{"name":"x"}`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatalf("without X-Bookmarks: %d", w.Code)
	}
}
