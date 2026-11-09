package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/danielfrg/bookmarks/internal/bookmarks"
	"github.com/danielfrg/bookmarks/internal/query"
)

func TestArchiveRowsAndBasicFlag(t *testing.T) {
	s, ctx := openTest(t), context.Background()
	id := addBM(t, s, 1, "https://a.test", "A")
	a := &bookmarks.Archive{BookmarkID: id, Source: bookmarks.SourceServer, ContentType: "text/html", FileName: "1/1-server.gz", DisplayName: "page", FileSize: 5}
	if err := s.AddArchive(ctx, a); err != nil || a.ID == 0 || a.CreatedAt.IsZero() {
		t.Fatalf("AddArchive: %v %+v", err, a)
	}
	b, _ := s.GetBookmark(ctx, id)
	if b.ArchiveCount != 1 || !b.BasicCaptureOnly {
		t.Fatalf("after server: %+v", b)
	}
	a2 := &bookmarks.Archive{BookmarkID: id, Source: bookmarks.SourceExtension, ContentType: "text/html", FileName: "1/2-extension.gz", CreatedAt: time.Now().Add(time.Hour)}
	if err := s.AddArchive(ctx, a2); err != nil {
		t.Fatal(err)
	}
	b, _ = s.GetBookmark(ctx, id)
	if b.ArchiveCount != 2 || b.BasicCaptureOnly {
		t.Fatalf("after extension: %+v", b)
	}
	list, err := s.ListArchives(ctx, id)
	if err != nil || len(list) != 2 || list[0].ID != a2.ID {
		t.Fatalf("list newest first: %v %+v", err, list)
	}
	got, err := s.GetArchive(ctx, a.ID)
	if err != nil || got.FileName != "1/1-server.gz" || got.DisplayName != "page" || got.FileSize != 5 || got.Source != bookmarks.SourceServer {
		t.Fatalf("get: %v %+v", err, got)
	}
	if _, err := s.GetArchive(ctx, 999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get missing: %v", err)
	}
	if err := s.DeleteArchive(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteArchive(ctx, a.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete missing: %v", err)
	}
	b, _ = s.GetBookmark(ctx, id)
	if b.ArchiveCount != 1 {
		t.Fatalf("count after delete: %d", b.ArchiveCount)
	}
}

func TestLatestReadableArchive(t *testing.T) {
	s, ctx := openTest(t), context.Background()
	id := addBM(t, s, 1, "https://a.test", "A")
	if _, err := s.LatestReadableArchive(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("empty: %v", err)
	}
	base := time.Now().UTC()
	html := &bookmarks.Archive{BookmarkID: id, Source: bookmarks.SourceServer, ContentType: "text/html", CreatedAt: base}
	png := &bookmarks.Archive{BookmarkID: id, Source: bookmarks.SourceUpload, ContentType: "image/png", CreatedAt: base.Add(time.Minute)}
	for _, a := range []*bookmarks.Archive{html, png} {
		if err := s.AddArchive(ctx, a); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.LatestReadableArchive(ctx, id)
	if err != nil || got.ID != html.ID {
		t.Fatalf("got %+v %v", got, err)
	}
	pdf := &bookmarks.Archive{BookmarkID: id, Source: bookmarks.SourceUpload, ContentType: "application/pdf", CreatedAt: base.Add(2 * time.Minute)}
	s.AddArchive(ctx, pdf)
	if got, _ = s.LatestReadableArchive(ctx, id); got.ID != pdf.ID {
		t.Fatalf("pdf should win: %+v", got)
	}
}

func TestReaderTextUpsertAndSearch(t *testing.T) {
	s, ctx := openTest(t), context.Background()
	id := addBM(t, s, 1, "https://a.test", "A")
	rt, err := s.GetReaderText(ctx, id)
	if err != nil || rt.Status != bookmarks.ReaderNone || rt.BookmarkID != id {
		t.Fatalf("absent: %v %+v", err, rt)
	}
	arc := &bookmarks.Archive{BookmarkID: id, Source: bookmarks.SourceServer, ContentType: "text/html"}
	s.AddArchive(ctx, arc)
	err = s.PutReaderText(ctx, bookmarks.ReaderText{BookmarkID: id, ArchiveID: arc.ID, Status: bookmarks.ReaderOK, Title: "T", Markdown: "zanzibar content", WordCount: 2})
	if err != nil {
		t.Fatal(err)
	}
	search := func(q string) int {
		res, err := s.Search(ctx, SearchQuery{Expr: query.Term{Text: q}})
		if err != nil {
			t.Fatal(err)
		}
		return res.Total
	}
	if search("zanzibar") != 1 {
		t.Fatal("reader text not searchable")
	}
	rt, _ = s.GetReaderText(ctx, id)
	if rt.Status != bookmarks.ReaderOK || rt.ArchiveID != arc.ID || rt.Markdown != "zanzibar content" || rt.WordCount != 2 || rt.ExtractedAt.IsZero() {
		t.Fatalf("get: %+v", rt)
	}
	if b, _ := s.GetBookmark(ctx, id); b.ReaderStatus != bookmarks.ReaderOK {
		t.Fatalf("bookmark reader status %q", b.ReaderStatus)
	}
	// Upsert replaces the indexed text.
	s.PutReaderText(ctx, bookmarks.ReaderText{BookmarkID: id, Status: bookmarks.ReaderOK, Markdown: "quokka"})
	if search("zanzibar") != 0 || search("quokka") != 1 {
		t.Fatal("reindex after upsert failed")
	}
	// Deleting the archive keeps the reader text, with archive id cleared.
	rt, _ = s.GetReaderText(ctx, id)
	s.PutReaderText(ctx, bookmarks.ReaderText{BookmarkID: id, ArchiveID: arc.ID, Status: bookmarks.ReaderOK, Markdown: "quokka"})
	if err := s.DeleteArchive(ctx, arc.ID); err != nil {
		t.Fatal(err)
	}
	rt, _ = s.GetReaderText(ctx, id)
	if rt.ArchiveID != 0 || rt.Markdown != "quokka" {
		t.Fatalf("after archive delete: %+v", rt)
	}
}
