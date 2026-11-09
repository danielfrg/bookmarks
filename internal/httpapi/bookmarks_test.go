package httpapi

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danielfrg/bookmarks/internal/bookmarks"
)

func TestCreateThenUpsert(t *testing.T) {
	e := newTestEnv(t)
	w := e.do("POST", "/api/bookmarks?capture=none", map[string]any{"url": "https://example.com/a", "title": "One", "tags": []string{"Go", "web"}})
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	first := decode[bookmarkResp](t, w)
	if first.ID == 0 || first.Title != "One" || len(first.Tags) != 2 || first.ReaderStatus != "none" {
		t.Fatalf("%+v", first)
	}
	w = e.do("POST", "/api/bookmarks?capture=none", map[string]any{"url": "https://example.com/a", "title": "Two"})
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	second := decode[bookmarkResp](t, w)
	if second.ID != first.ID || second.Title != "Two" || len(second.Tags) != 2 {
		t.Fatalf("%+v", second)
	}
}

func TestBookmarkCRUDAndErrors(t *testing.T) {
	e := newTestEnv(t)
	b := e.create(t, "https://example.com/a")
	path := fmt.Sprintf("/api/bookmarks/%d", b.ID)

	if w := e.do("GET", path, nil); w.Code != 200 {
		t.Fatal(w.Code)
	}
	w := e.do("PATCH", path, map[string]any{"unread": true, "tags": []string{"x"}})
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	got := decode[bookmarkResp](t, w)
	if !got.Unread || got.Title != "T" || len(got.Tags) != 1 {
		t.Fatalf("%+v", got)
	}
	if w := e.do("GET", "/api/bookmarks/9999", nil); w.Code != 404 {
		t.Fatal(w.Code)
	}
	if w := e.do("POST", "/api/bookmarks", map[string]any{"url": "ftp://nope"}); w.Code != 422 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := e.do("POST", "/api/bookmarks?capture=bogus", map[string]any{"url": "https://example.com/z"}); w.Code != 422 {
		t.Fatal(w.Code, w.Body.String())
	}
	other := e.create(t, "https://example.com/b")
	w = e.do("PATCH", fmt.Sprintf("/api/bookmarks/%d", other.ID), map[string]any{"url": "https://example.com/a"})
	if w.Code != 409 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := e.do("DELETE", path, nil); w.Code != 204 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := e.do("GET", path, nil); w.Code != 404 {
		t.Fatal(w.Code)
	}
}

func TestListShape(t *testing.T) {
	e := newTestEnv(t)
	e.create(t, "https://example.com/a")
	e.create(t, "https://example.com/b")
	w := e.do("GET", "/api/bookmarks?limit=1", nil)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	type listResp struct {
		Count   int `json:"count"`
		Results []struct {
			bookmarkResp
			Snippet *string `json:"snippet"`
		} `json:"results"`
	}
	res := decode[listResp](t, w)
	if res.Count != 2 || len(res.Results) != 1 || res.Results[0].Snippet == nil {
		t.Fatalf("%+v body=%s", res, w.Body)
	}
	for _, k := range []string{`"favicon_url":""`, `"preview_image_url":""`, `"web_archive_url":""`, `"is_archived":false`, `"date_modified"`, `"notes"`, `"description"`} {
		if !strings.Contains(w.Body.String(), k) {
			t.Errorf("missing %s in %s", k, w.Body)
		}
	}
	if w := e.do("GET", "/api/bookmarks?q=example&unread=maybe", nil); w.Code != 422 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := e.do("GET", "/api/bookmarks?q=b", nil); w.Code != 200 || !strings.Contains(w.Body.String(), `"count":1`) {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestCheckEndpoint(t *testing.T) {
	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><head><title>Page title</title><meta name="description" content="Desc"></head></html>`)
	}))
	defer page.Close()
	e := newTestEnv(t)
	u := page.URL + "/page"
	w := e.do("GET", "/api/bookmarks/check?url="+u, nil)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, s := range []string{`"bookmark":null`, `"basic_capture_only":false`, `"title":"Page title"`, `"description":"Desc"`, `"preview_image_url":""`, `"auto_tags":[]`} {
		if !strings.Contains(w.Body.String(), s) {
			t.Errorf("missing %s in %s", s, w.Body)
		}
	}
	b := e.create(t, u)
	w = e.do("GET", "/api/bookmarks/check?url="+u, nil)
	if !strings.Contains(w.Body.String(), fmt.Sprintf(`"id":%d`, b.ID)) {
		t.Fatal(w.Body.String())
	}
	if w := e.do("GET", "/api/bookmarks/check?url=nope", nil); w.Code != 422 {
		t.Fatal(w.Code)
	}
}

func TestReaderEndpoint(t *testing.T) {
	e := newTestEnv(t)
	b := e.create(t, "https://example.com/a")
	w := e.do("GET", fmt.Sprintf("/api/bookmarks/%d/reader", b.ID), nil)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, s := range []string{`"status":"none"`, `"markdown":""`, `"error":""`, `"offset":0`, `"total_chars":0`, `"next_offset":null`, `"word_count":0`, `"extracted_at":null`} {
		if !strings.Contains(w.Body.String(), s) {
			t.Errorf("missing %s in %s", s, w.Body)
		}
	}
	if w := e.do("GET", fmt.Sprintf("/api/bookmarks/%d/reader?offset=5", b.ID), nil); w.Code != 422 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := e.do("GET", "/api/bookmarks/999/reader", nil); w.Code != 404 {
		t.Fatal(w.Code)
	}
}

func TestCaptureAndExtractAccepted(t *testing.T) {
	e := newTestEnv(t)
	b := e.create(t, "https://example.com/a")
	for _, p := range []string{"capture", "reader/extract"} {
		if w := e.do("POST", fmt.Sprintf("/api/bookmarks/%d/%s", b.ID, p), nil); w.Code != 202 {
			t.Fatal(p, w.Code, w.Body.String())
		}
		if w := e.do("POST", fmt.Sprintf("/api/bookmarks/%d/%s", 999, p), nil); w.Code != 404 {
			t.Fatal(p, w.Code)
		}
	}
}

func TestToBookmarkImageURLs(t *testing.T) {
	j := toBookmark(bookmarks.Bookmark{FaviconFile: "f.png", PreviewFile: "p.jpg"})
	if j.FaviconURL != "/favicons/f.png" || j.PreviewImageURL != "/previews/p.jpg" || j.Tags == nil || j.ReaderStatus != "none" {
		t.Fatalf("%+v", j)
	}
}

func TestUnknownFieldsAccepted(t *testing.T) {
	e := newTestEnv(t)
	w := e.do("POST", "/api/bookmarks?capture=none", map[string]any{"url": "https://example.com/x", "future_field": 1})
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	id := decode[bookmarkResp](t, w).ID
	if w := e.do("PATCH", fmt.Sprintf("/api/bookmarks/%d", id), map[string]any{"title": "n", "future_field": []int{1}}); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
}
