package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/danielfrg/bookmarks/internal/bookmarks"
	"github.com/danielfrg/bookmarks/internal/jobs"
	"github.com/danielfrg/bookmarks/internal/store"
)

func TestSaveCreatesAndEnqueues(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	b, created, err := e.svc.Save(ctx, SaveInput{URL: "https://example.com/a", Title: ptr("A")})
	if err != nil || !created {
		t.Fatalf("Save = %v, created=%v", err, created)
	}
	if b.ID == 0 || b.Title != "A" {
		t.Fatalf("bookmark = %+v", b)
	}
	if got, want := e.q.kinds(b.ID), []jobs.Kind{jobs.KindMetadata, jobs.KindCapture}; !reflect.DeepEqual(got, want) {
		t.Fatalf("jobs = %v, want %v", got, want)
	}

	b2, _, err := e.svc.Save(ctx, SaveInput{URL: "https://example.com/b", Capture: CaptureNone})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := e.q.kinds(b2.ID), []jobs.Kind{jobs.KindMetadata}; !reflect.DeepEqual(got, want) {
		t.Fatalf("jobs = %v, want %v", got, want)
	}
}

func TestSaveEnqueuesWaybackWhenEnabled(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	s := bookmarks.DefaultSettings()
	s.WaybackEnabled = true
	if _, err := e.svc.PutSettings(ctx, s); err != nil {
		t.Fatal(err)
	}
	b, _, err := e.svc.Save(ctx, SaveInput{URL: "https://example.com/a", Capture: CaptureNone})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := e.q.kinds(b.ID), []jobs.Kind{jobs.KindMetadata, jobs.KindWayback}; !reflect.DeepEqual(got, want) {
		t.Fatalf("jobs = %v, want %v", got, want)
	}
}

func TestSaveValidation(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	for _, in := range []SaveInput{
		{URL: ""},
		{URL: "ftp://example.com"},
		{URL: "https://example.com", Capture: "bogus"},
	} {
		_, _, err := e.svc.Save(ctx, in)
		var ve *ValidationError
		if !errors.As(err, &ve) {
			t.Fatalf("Save(%+v) err = %v, want ValidationError", in, err)
		}
	}
	_, _, err := e.svc.Save(ctx, SaveInput{URL: ""})
	if got := err.Error(); got != "url: is required" {
		t.Fatalf("message = %q", got)
	}
}

func TestSaveBookmarkMergesNormalizedURL(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	first, _, err := e.svc.Save(ctx, SaveInput{
		URL: "https://Example.com/path/?b=2&a=1", Title: ptr("Title"), Notes: ptr("notes"),
		Tags: &[]string{"Go", "web"},
	})
	if err != nil {
		t.Fatal(err)
	}
	e.q.reset()

	second, created, err := e.svc.Save(ctx, SaveInput{
		URL: "https://example.com/path?a=1&b=2", Description: ptr("desc"), Capture: CaptureNone,
	})
	if err != nil {
		t.Fatal(err)
	}
	if created || second.ID != first.ID {
		t.Fatalf("created=%v id=%d, want existing %d", created, second.ID, first.ID)
	}
	if second.Title != "Title" || second.Notes != "notes" || second.Description != "desc" {
		t.Fatalf("merged = %+v", second)
	}
	if !reflect.DeepEqual(second.Tags, []string{"go", "web"}) {
		t.Fatalf("tags = %v", second.Tags)
	}
	if second.URL != first.URL {
		t.Fatalf("url changed to %q", second.URL)
	}
	res, err := e.svc.Search(ctx, SearchInput{Scope: "all"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 1 {
		t.Fatalf("rows = %d, want 1", res.Total)
	}
	if got := e.q.kinds(first.ID); len(got) != 0 {
		t.Fatalf("existing save with Capture none enqueued %v", got)
	}

	// Default capture mode on an existing bookmark without archives → capture.
	if _, _, err := e.svc.Save(ctx, SaveInput{URL: first.URL, Tags: &[]string{"A,B"}}); err != nil {
		t.Fatal(err)
	}
	if got, want := e.q.kinds(first.ID), []jobs.Kind{jobs.KindCapture}; !reflect.DeepEqual(got, want) {
		t.Fatalf("jobs = %v, want %v", got, want)
	}
	got, _ := e.svc.Get(ctx, first.ID)
	if !reflect.DeepEqual(got.Tags, []string{"a", "b"}) {
		t.Fatalf("tags = %v", got.Tags)
	}
}

func TestSaveAppliesAutoTagsAndDefaultUnread(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	s := bookmarks.DefaultSettings()
	s.DefaultUnread = true
	s.AutoTagRules = "github.com code\nexample.com other"
	if _, err := e.svc.PutSettings(ctx, s); err != nil {
		t.Fatal(err)
	}
	b, _, err := e.svc.Save(ctx, SaveInput{URL: "https://github.com/danielfrg", Tags: &[]string{"Mine"}})
	if err != nil {
		t.Fatal(err)
	}
	if !b.Unread {
		t.Fatal("Unread = false, want settings default true")
	}
	if !reflect.DeepEqual(b.Tags, []string{"code", "mine"}) {
		t.Fatalf("tags = %v", b.Tags)
	}
	b2, _, err := e.svc.Save(ctx, SaveInput{URL: "https://github.com/x", Unread: ptr(false)})
	if err != nil {
		t.Fatal(err)
	}
	if b2.Unread {
		t.Fatal("explicit Unread=false ignored")
	}
}

func TestSaveKeepsImportedDates(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	added := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	modified := time.Date(2021, 6, 7, 8, 9, 10, 0, time.UTC)
	b, _, err := e.svc.Save(ctx, SaveInput{URL: "https://example.com", DateAdded: &added, DateModified: &modified, Archived: ptr(true)})
	if err != nil {
		t.Fatal(err)
	}
	if !b.DateAdded.Equal(added) || !b.DateModified.Equal(modified) || !b.Archived {
		t.Fatalf("got added=%v modified=%v archived=%v", b.DateAdded, b.DateModified, b.Archived)
	}
	b2, _, err := e.svc.Save(ctx, SaveInput{URL: "https://example.org"})
	if err != nil {
		t.Fatal(err)
	}
	if !b2.DateAdded.Equal(fixedNow) || !b2.DateModified.Equal(fixedNow) {
		t.Fatalf("defaults = %v / %v, want now", b2.DateAdded, b2.DateModified)
	}
}

func TestUpdatePatchesAndConflicts(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a, _, _ := e.svc.Save(ctx, SaveInput{URL: "https://a.example", Title: ptr("A"), Notes: ptr("n")})
	_, _, _ = e.svc.Save(ctx, SaveInput{URL: "https://b.example"})

	got, err := e.svc.Update(ctx, a.ID, SaveInput{Title: ptr("A2"), Tags: &[]string{"X Y"}})
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "A2" || got.Notes != "n" || !reflect.DeepEqual(got.Tags, []string{"x", "y"}) {
		t.Fatalf("updated = %+v", got)
	}
	if _, err := e.svc.Update(ctx, a.ID, SaveInput{URL: "https://B.example/"}); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("err = %v, want ErrConflict", err)
	}
	var ve *ValidationError
	if _, err := e.svc.Update(ctx, a.ID, SaveInput{URL: "nope"}); !errors.As(err, &ve) {
		t.Fatalf("err = %v, want ValidationError", err)
	}
	if _, err := e.svc.Update(ctx, 999, SaveInput{}); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestSearchWithSavedSearch(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	mk := func(url string, tags []string, unread bool) int64 {
		b, _, err := e.svc.Save(ctx, SaveInput{URL: url, Title: ptr("golang " + url), Tags: &tags, Unread: &unread})
		if err != nil {
			t.Fatal(err)
		}
		return b.ID
	}
	want := mk("https://a.example", []string{"go", "web"}, true)
	mk("https://b.example", []string{"go", "web", "old"}, true) // excluded tag
	mk("https://c.example", []string{"go"}, true)               // missing all-tag
	mk("https://d.example", []string{"rust", "web"}, true)      // no any-tag
	mk("https://e.example", []string{"go", "web"}, false)       // read
	mk("https://f.example", []string{"python", "web"}, true)    // any-tag python

	ss, err := e.svc.SaveSavedSearch(ctx, bookmarks.SavedSearch{
		Name: "Go web", Query: "golang", AnyTags: []string{"Go", "python"}, AllTags: []string{"WEB"},
		ExcludedTags: []string{"old"}, FilterUnread: "yes",
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := e.svc.Search(ctx, SearchInput{SavedSearchID: ss.ID, Q: "not #python"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 1 || res.Hits[0].ID != want {
		t.Fatalf("hits = %+v", res)
	}
	res, err = e.svc.Search(ctx, SearchInput{SavedSearchID: ss.ID})
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 2 {
		t.Fatalf("total = %d, want 2 (a and f)", res.Total)
	}
	if _, err := e.svc.Search(ctx, SearchInput{SavedSearchID: 999}); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestSearchValidation(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	for _, in := range []SearchInput{
		{Q: "(unclosed"},
		{Q: strings.Repeat("(", 1025)},
		{Sort: "bogus"},
		{Scope: "bogus"},
		{Unread: "maybe"},
	} {
		_, err := e.svc.Search(ctx, in)
		var ve *ValidationError
		if !errors.As(err, &ve) {
			t.Fatalf("Search(%.20q) err = %v, want ValidationError", in.Q, err)
		}
	}
	_, err := e.svc.Search(ctx, SearchInput{Q: "(unclosed"})
	var ve *ValidationError
	errors.As(err, &ve)
	if ve.Field != "q" {
		t.Fatalf("field = %q", ve.Field)
	}
}

func TestSavedSearchValidation(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	for _, b := range []bookmarks.SavedSearch{
		{Name: " "},
		{Name: "x", FilterUnread: "maybe"},
		{Name: "x", Query: "(bad"},
	} {
		var ve *ValidationError
		if _, err := e.svc.SaveSavedSearch(ctx, b); !errors.As(err, &ve) {
			t.Fatalf("SaveSavedSearch(%+v) err = %v", b, err)
		}
	}
	b, err := e.svc.SaveSavedSearch(ctx, bookmarks.SavedSearch{Name: "x", AnyTags: []string{"B a", "A"}})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.AnyTags, []string{"a", "b"}) || b.FilterUnread != "off" || b.Order != 1 {
		t.Fatalf("saved search = %+v", b)
	}
	b.Name = "y"
	b2, err := e.svc.SaveSavedSearch(ctx, b)
	if err != nil || b2.Name != "y" || b2.Order != 1 {
		t.Fatalf("update = %+v, %v", b2, err)
	}
}

func TestTagsRename(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	if _, _, err := e.svc.Save(ctx, SaveInput{URL: "https://a.example", Tags: &[]string{"old"}}); err != nil {
		t.Fatal(err)
	}
	tags, err := e.svc.Tags(ctx)
	if err != nil || len(tags) != 1 {
		t.Fatalf("tags = %v, %v", tags, err)
	}
	for _, name := range []string{"", "  ", "a b", "a,b"} {
		var ve *ValidationError
		if err := e.svc.RenameTag(ctx, tags[0].ID, name); !errors.As(err, &ve) {
			t.Fatalf("RenameTag(%q) err = %v", name, err)
		}
	}
	if err := e.svc.RenameTag(ctx, tags[0].ID, "New"); err != nil {
		t.Fatal(err)
	}
	tags, _ = e.svc.Tags(ctx)
	if tags[0].Name != "new" {
		t.Fatalf("tags = %+v", tags)
	}
	if err := e.svc.DeleteTag(ctx, tags[0].ID); err != nil {
		t.Fatal(err)
	}
}

func TestCheck(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if strings.HasPrefix(r.URL.Path, "/broken") {
			w.WriteHeader(500)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><head><title>Page title</title><meta name="description" content="Desc"></head></html>`)
	}))
	defer srv.Close()
	e := newEnv(t)
	ctx := context.Background()
	s := bookmarks.DefaultSettings()
	s.AutoTagRules = "127.0.0.1 local"
	if _, err := e.svc.PutSettings(ctx, s); err != nil {
		t.Fatal(err)
	}

	res, err := e.svc.Check(ctx, srv.URL+"/page")
	if err != nil {
		t.Fatal(err)
	}
	if res.Bookmark != nil || res.Metadata.Title != "Page title" || res.Metadata.Description != "Desc" {
		t.Fatalf("check = %+v", res)
	}
	if !reflect.DeepEqual(res.AutoTags, []string{"local"}) {
		t.Fatalf("auto tags = %v", res.AutoTags)
	}
	if _, err := e.svc.Check(ctx, srv.URL+"/page"); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 1 {
		t.Fatalf("server hits = %d, want 1 (cached)", hits.Load())
	}

	b, _, _ := e.svc.Save(ctx, SaveInput{URL: srv.URL + "/broken"})
	if _, err := e.svc.AddArchive(ctx, b.ID, ArchiveInput{Source: bookmarks.SourceServer, ContentType: "text/html", Body: strings.NewReader("x")}); err != nil {
		t.Fatal(err)
	}
	res, err = e.svc.Check(ctx, srv.URL+"/broken/")
	if err != nil {
		t.Fatal(err)
	}
	if res.Bookmark == nil || res.Bookmark.ID != b.ID || !res.BasicCaptureOnly || res.Metadata.Title != "" {
		t.Fatalf("check = %+v", res)
	}
	var ve *ValidationError
	if _, err := e.svc.Check(ctx, "nope"); !errors.As(err, &ve) {
		t.Fatalf("err = %v", err)
	}
}

func TestFindByURL(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	if _, err := e.svc.FindByURL(ctx, "https://example.com/f"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("got %v", err)
	}
	saved, _, err := e.svc.Save(ctx, SaveInput{URL: "https://example.com/f", Title: ptr("F")})
	if err != nil {
		t.Fatal(err)
	}
	b, err := e.svc.FindByURL(ctx, "https://example.com/f")
	if err != nil || b.ID != saved.ID {
		t.Fatalf("%+v %v", b, err)
	}
	var ve *ValidationError
	if _, err := e.svc.FindByURL(ctx, "ftp://x"); !errors.As(err, &ve) {
		t.Fatalf("got %v", err)
	}
}

// blindStore hides the existing bookmark from the first URL lookup, as if a
// concurrent save created it between the lookup and the insert.
type blindStore struct {
	Store
	calls int
}

func (b *blindStore) GetBookmarkByURL(ctx context.Context, u string) (bookmarks.Bookmark, error) {
	b.calls++
	if b.calls == 1 {
		return bookmarks.Bookmark{}, store.ErrNotFound
	}
	return b.Store.GetBookmarkByURL(ctx, u)
}

func TestSaveConcurrentCreateMergesIntoExisting(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	first, _, err := e.svc.Save(ctx, SaveInput{URL: "https://race.example/", Title: ptr("one"), Capture: CaptureNone})
	if err != nil {
		t.Fatal(err)
	}
	e.svc.st = &blindStore{Store: e.svc.st}
	b, created, err := e.svc.Save(ctx, SaveInput{URL: "https://race.example/", Notes: ptr("two"), Capture: CaptureNone})
	if err != nil || created {
		t.Fatalf("created=%v err=%v, want merge", created, err)
	}
	if b.ID != first.ID || b.Title != "one" || b.Notes != "two" {
		t.Fatalf("merged = %+v", b)
	}
}

func TestDeleteRemovesArchiveDirectory(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	b, _, _ := e.svc.Save(ctx, SaveInput{URL: "https://d.example/", Capture: CaptureNone})
	addFileArchive(t, e, b.ID, "text/html", "../reader/testdata/article.html")
	dir := filepath.Join(e.dir, "archives", strconv.FormatInt(b.ID, 10))
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("archive dir missing before delete: %v", err)
	}
	if err := e.svc.Delete(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("archive dir still exists: %v", err)
	}
}
