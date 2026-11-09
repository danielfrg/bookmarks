package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

type tagResp struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type savedSearchResp struct {
	ID           int64    `json:"id"`
	Name         string   `json:"name"`
	Query        string   `json:"query"`
	AnyTags      []string `json:"any_tags"`
	AllTags      []string `json:"all_tags"`
	ExcludedTags []string `json:"excluded_tags"`
	FilterUnread string   `json:"filter_unread"`
	Order        int      `json:"order"`
	DateCreated  string   `json:"date_created"`
}

func TestTagsSavedSearchesSettingsEndpoints(t *testing.T) {
	e := newTestEnv(t)
	for _, u := range []string{"https://a.example", "https://b.example"} {
		if w := e.do("POST", "/_/api/bookmarks?capture=none", map[string]any{"url": u, "tags": []string{"go", "web"}}); w.Code != 201 {
			t.Fatal(w.Code, w.Body.String())
		}
	}

	// Tags.
	tags := decode[[]tagResp](t, e.do("GET", "/_/api/tags", nil))
	if len(tags) != 2 || tags[0].Name != "go" || tags[0].Count != 2 {
		t.Fatalf("tags = %+v", tags)
	}
	w := e.do("PATCH", "/_/api/tags/"+itoa(tags[0].ID), map[string]any{"name": "golang", "extra": 1})
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if got := decode[tagResp](t, w); got.Name != "golang" || got.Count != 2 || got.ID != tags[0].ID {
		t.Fatalf("renamed = %+v", got)
	}
	// Renaming onto an existing name merges and returns the surviving tag.
	w = e.do("PATCH", "/_/api/tags/"+itoa(tags[0].ID), map[string]any{"name": "web"})
	if got := decode[tagResp](t, w); w.Code != 200 || got.Name != "web" || got.Count != 2 {
		t.Fatalf("merged = %d %+v", w.Code, got)
	}
	if w := e.do("PATCH", "/_/api/tags/999", map[string]any{"name": "x"}); w.Code != 404 {
		t.Fatal(w.Code)
	}
	if w := e.do("DELETE", "/_/api/tags/"+itoa(tags[1].ID), nil); w.Code != 204 {
		t.Fatal(w.Code, w.Body.String())
	}
	if got := decode[[]tagResp](t, e.do("GET", "/_/api/tags", nil)); len(got) != 0 {
		t.Fatalf("tags after delete = %+v", got)
	}

	// SavedSearches.
	w = e.do("POST", "/_/api/saved-searches", map[string]any{"name": "Reading", "any_tags": []string{"Go"}, "filter_unread": "yes", "unknown": true})
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	b1 := decode[savedSearchResp](t, w)
	if b1.Name != "Reading" || b1.FilterUnread != "yes" || len(b1.AnyTags) != 1 || b1.AnyTags[0] != "go" || b1.AllTags == nil || b1.DateCreated == "" {
		t.Fatalf("saved search = %+v", b1)
	}
	b2 := decode[savedSearchResp](t, e.do("POST", "/_/api/saved-searches", map[string]any{"name": "Second"}))
	if b2.FilterUnread != "off" {
		t.Fatalf("default filter_unread = %q", b2.FilterUnread)
	}
	if w := e.do("POST", "/_/api/saved-searches", map[string]any{"name": "Bad", "filter_unread": "maybe"}); w.Code != 422 {
		t.Fatal("bad filter_unread:", w.Code, w.Body.String())
	}
	w = e.do("PUT", "/_/api/saved-searches/"+itoa(b1.ID), map[string]any{"name": "Renamed", "query": "golang", "filter_unread": "no"})
	if got := decode[savedSearchResp](t, w); w.Code != 200 || got.Name != "Renamed" || got.Query != "golang" || got.FilterUnread != "no" {
		t.Fatalf("put = %d %+v", w.Code, got)
	}
	if w := e.do("PUT", "/_/api/saved-searches/"+itoa(b1.ID), map[string]any{"name": "R", "filter_unread": "x"}); w.Code != 422 {
		t.Fatal("put bad filter_unread:", w.Code)
	}
	if w := e.do("PUT", "/_/api/saved-searches/999", map[string]any{"name": "R"}); w.Code != 404 {
		t.Fatal(w.Code)
	}
	if w := e.do("POST", "/_/api/saved-searches/reorder", map[string]any{"ids": []int64{b2.ID, b1.ID}}); w.Code != 204 {
		t.Fatal(w.Code, w.Body.String())
	}
	list := decode[[]savedSearchResp](t, e.do("GET", "/_/api/saved-searches", nil))
	if len(list) != 2 || list[0].ID != b2.ID {
		t.Fatalf("order = %+v", list)
	}
	if w := e.do("GET", "/_/api/saved-searches/"+itoa(b1.ID), nil); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w := e.do("DELETE", "/_/api/saved-searches/"+itoa(b1.ID), nil); w.Code != 204 {
		t.Fatal(w.Code)
	}
	if w := e.do("GET", "/_/api/saved-searches/"+itoa(b1.ID), nil); w.Code != 404 {
		t.Fatal(w.Code)
	}

	// Settings.
	w = e.do("GET", "/_/api/settings", nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"ui":{}`) || !strings.Contains(w.Body.String(), `"favicons_enabled":true`) {
		t.Fatal(w.Code, w.Body.String())
	}
	w = e.do("PUT", "/_/api/settings", map[string]any{
		"favicons_enabled": false, "previews_enabled": true, "wayback_enabled": true, "default_unread": true,
		"auto_tag_rules": "example.com news", "ui": map[string]any{"view": "grid", "n": 3},
	})
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	got := decode[struct {
		Favicons bool           `json:"favicons_enabled"`
		Wayback  bool           `json:"wayback_enabled"`
		Unread   bool           `json:"default_unread"`
		Rules    string         `json:"auto_tag_rules"`
		UI       map[string]any `json:"ui"`
	}](t, e.do("GET", "/_/api/settings", nil))
	if got.Favicons || !got.Wayback || !got.Unread || got.Rules != "example.com news" || got.UI["view"] != "grid" {
		t.Fatalf("settings = %+v", got)
	}
	for _, ui := range []any{"str", []int{1}, 5, nil} {
		if w := e.do("PUT", "/_/api/settings", map[string]any{"ui": ui}); w.Code != 422 {
			t.Errorf("ui=%v: status %d %s", ui, w.Code, w.Body.String())
		}
	}
}

func TestBulkByQuery(t *testing.T) {
	e := newTestEnv(t)
	mk := func(u string, tags ...string) bookmarkResp {
		w := e.do("POST", "/_/api/bookmarks?capture=none", map[string]any{"url": u, "tags": tags})
		return decode[bookmarkResp](t, w)
	}
	a := mk("https://a.example", "keep")
	mk("https://b.example", "keep")
	mk("https://c.example", "other")

	w := e.do("POST", "/_/api/bookmarks/bulk", map[string]any{"query": map[string]any{"q": "#keep"}, "action": "archive"})
	if w.Code != 200 || decode[struct{ Affected int }](t, w).Affected != 2 {
		t.Fatal(w.Code, w.Body.String())
	}
	if got := decode[struct{ Count int }](t, e.do("GET", "/_/api/bookmarks?scope=archived", nil)); got.Count != 2 {
		t.Fatalf("archived = %d", got.Count)
	}
	w = e.do("POST", "/_/api/bookmarks/bulk", map[string]any{"ids": []int64{a.ID, 9999}, "action": "tag", "tags": []string{"Picked"}})
	if got := decode[struct {
		Affected int `json:"affected"`
	}](t, w); w.Code != 200 || got.Affected != 1 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := e.do("POST", "/_/api/bookmarks/bulk", map[string]any{"ids": []int64{a.ID}, "action": "explode"}); w.Code != 422 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := e.do("POST", "/_/api/bookmarks/bulk", map[string]any{"action": "archive"}); w.Code != 422 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := e.do("POST", "/_/api/bookmarks/bulk", map[string]any{"query": map[string]any{"scope": "bogus"}, "action": "archive"}); w.Code != 422 {
		t.Fatal(w.Code, w.Body.String())
	}
}

func importReq(e *testEnv, content string) *httptest.ResponseRecorder {
	ct, body := multipartBody([][3]string{{"file", "bookmarks.html", content}})
	r := httptest.NewRequest("POST", "/_/api/import/netscape", body)
	r.Header.Set("Content-Type", ct)
	r.Header.Set("X-Bookmarks", "1")
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, r)
	return w
}

func TestNetscapeImportExportEndpoints(t *testing.T) {
	e := newTestEnv(t)
	w := importReq(e, `<DL><p><DT><A HREF="https://a.example/" ADD_DATE="1616337559" TAGS="x,linkding:bookmarks.archived" TOREAD="1">A &amp; B</A>
<DD>desc[linkding-notes]n[/linkding-notes]
<DT><A HREF="nope">bad</A></DL>`)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if got := decode[struct{ Created, Updated, Skipped int }](t, w); got.Created != 1 || got.Skipped != 1 {
		t.Fatalf("result = %+v (%s)", got, w.Body.String())
	}

	w = e.do("GET", "/_/api/export/netscape", nil)
	if w.Code != 200 || w.Header().Get("Content-Type") != "text/html; charset=utf-8" ||
		w.Header().Get("Content-Disposition") != `attachment; filename="bookmarks.html"` {
		t.Fatal(w.Code, w.Header())
	}
	body := w.Body.String()
	for _, want := range []string{"<!DOCTYPE NETSCAPE-Bookmark-file-1>", `HREF="https://a.example/"`, "A &amp; B", "x,linkding:bookmarks.archived", `TOREAD="1"`, "[linkding-notes]n[/linkding-notes]"} {
		if !strings.Contains(body, want) {
			t.Errorf("export lacks %q:\n%s", want, body)
		}
	}

	// No file part.
	ct, b := multipartBody([][3]string{{"other", "", "x"}})
	r := httptest.NewRequest("POST", "/_/api/import/netscape", b)
	r.Header.Set("Content-Type", ct)
	r.Header.Set("X-Bookmarks", "1")
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, r)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatal(rec.Code, rec.Body.String())
	}
}

func TestNetscapeImportTooLarge(t *testing.T) {
	e := newTestEnv(t)
	w := importReq(e, "<DL>"+strings.Repeat("x", 17<<20))
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatal(w.Code, w.Body.String()[:min(200, w.Body.Len())])
	}
}

func TestExportNetscapeSearchFailureIsErrorNotTruncated(t *testing.T) {
	e := newTestEnv(t)
	e.create(t, "https://a.example/")
	e.st.Close() // every search now fails
	w := e.do("GET", "/_/api/export/netscape", nil)
	if w.Code != 500 {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	if strings.Contains(w.Body.String(), "NETSCAPE") || w.Header().Get("Content-Disposition") != "" {
		t.Fatalf("partial export: %v %q", w.Header(), w.Body.String())
	}
}
