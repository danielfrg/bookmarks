package store

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/danielfrg/bookmarks/internal/bookmarks"
)

func sample(url string, tags ...string) *bookmarks.Bookmark {
	return &bookmarks.Bookmark{
		URL: url, Title: "T", Description: "D", Notes: "N", Tags: tags,
		Unread: true, Archived: true, FaviconFile: "f.png", PreviewFile: "p.png", WebArchiveURL: "https://web.archive.org/x",
		DateAdded:    time.Date(2026, 1, 2, 3, 4, 5, 6, time.UTC),
		DateModified: time.Date(2026, 2, 3, 4, 5, 6, 7, time.UTC),
	}
}

func TestCreateGetBookmarkRoundTrip(t *testing.T) {
	s, ctx := openTest(t), context.Background()
	b := sample("https://example.com/a", "go", "api")
	if err := s.CreateBookmark(ctx, b); err != nil || b.ID == 0 {
		t.Fatalf("create: %v id=%d", err, b.ID)
	}
	got, err := s.GetBookmark(ctx, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := *b
	want.Tags = []string{"api", "go"}
	want.ReaderStatus = bookmarks.ReaderNone
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
	if got.ArchiveCount != 0 || got.BasicCaptureOnly {
		t.Fatal("unexpected archive info")
	}
	if _, err := s.GetBookmark(ctx, 999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestCreateBookmarkConflictOnNormalizedURL(t *testing.T) {
	s, ctx := openTest(t), context.Background()
	if err := s.CreateBookmark(ctx, sample("https://example.com/a")); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateBookmark(ctx, sample("https://EXAMPLE.com/a/")); !errors.Is(err, ErrConflict) {
		t.Fatalf("err = %v", err)
	}
}

func TestGetBookmarkByURL(t *testing.T) {
	s, ctx := openTest(t), context.Background()
	b := sample("https://example.com/a")
	s.CreateBookmark(ctx, b)
	got, err := s.GetBookmarkByURL(ctx, "https://EXAMPLE.com/a/")
	if err != nil || got.ID != b.ID {
		t.Fatalf("got %v err %v", got.ID, err)
	}
	if _, err := s.GetBookmarkByURL(ctx, "https://nope.com"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestUpdateBookmarkReplacesTags(t *testing.T) {
	s, ctx := openTest(t), context.Background()
	b := sample("https://example.com/a", "a", "b")
	s.CreateBookmark(ctx, b)
	b.Tags = []string{"b", "c"}
	b.Title = "New"
	b.Unread = false
	if err := s.UpdateBookmark(ctx, *b); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetBookmark(ctx, b.ID)
	if !reflect.DeepEqual(got.Tags, []string{"b", "c"}) || got.Title != "New" || got.Unread {
		t.Fatalf("got %+v", got)
	}
	b.ID = 999
	if err := s.UpdateBookmark(ctx, *b); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestUpdateBookmarkConflict(t *testing.T) {
	s, ctx := openTest(t), context.Background()
	s.CreateBookmark(ctx, sample("https://example.com/a"))
	b := sample("https://example.com/b")
	s.CreateBookmark(ctx, b)
	b.URL = "https://example.com/a/"
	if err := s.UpdateBookmark(ctx, *b); !errors.Is(err, ErrConflict) {
		t.Fatalf("err = %v", err)
	}
}

func TestDeleteBookmarkCascades(t *testing.T) {
	s, ctx := openTest(t), context.Background()
	b := sample("https://example.com/a", "keep")
	s.CreateBookmark(ctx, b)
	s.db.Exec(`INSERT INTO archives(bookmark_id,source,content_type,file_name,display_name,file_size,created_at) VALUES(?,?,?,?,?,?,?)`, b.ID, "server", "text/html", "f", "d", 1, "2026-01-01T00:00:00Z")
	s.db.Exec(`INSERT INTO jobs(kind,bookmark_id,status,run_at,created_at,updated_at) VALUES('x',?,'pending','a','a','a')`, b.ID)
	got, _ := s.GetBookmark(ctx, b.ID)
	if got.ArchiveCount != 1 || !got.BasicCaptureOnly {
		t.Fatalf("archive info: %+v", got)
	}
	if err := s.DeleteBookmark(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"bookmark_tags", "archives", "jobs"} {
		var n int
		s.db.QueryRow("SELECT COUNT(*) FROM " + q).Scan(&n)
		if n != 0 {
			t.Fatalf("%s rows remain", q)
		}
	}
	tags, _ := s.ListTags(ctx)
	if len(tags) != 1 || tags[0].Name != "keep" || tags[0].Count != 0 {
		t.Fatalf("tags = %+v", tags)
	}
	if err := s.DeleteBookmark(ctx, b.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}
