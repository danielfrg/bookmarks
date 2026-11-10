package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/danielfrg/bookmarks/internal/archive"
	"github.com/danielfrg/bookmarks/internal/bookmarks"
	"github.com/danielfrg/bookmarks/internal/jobs"
	"github.com/danielfrg/bookmarks/internal/store"
)

func TestAddArchiveEnqueuesExtract(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	b, _, _ := e.svc.Save(ctx, SaveInput{URL: "https://a.example", Capture: CaptureNone})
	e.q.reset()

	a, err := e.svc.AddArchive(ctx, b.ID, ArchiveInput{
		Source: bookmarks.SourceExtension, ContentType: "text/html; charset=utf-8",
		DisplayName: "SingleFile", Body: strings.NewReader("<html>hello</html>"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if a.ID == 0 || a.FileSize != 18 || a.ContentType != "text/html" || a.Source != bookmarks.SourceExtension {
		t.Fatalf("archive = %+v", a)
	}
	if !regexp.MustCompile(fmt.Sprintf(`^%d/%d-extension-[0-9a-f]{6}\.gz$`, b.ID, fixedNow.UnixNano())).MatchString(a.FileName) {
		t.Fatalf("file name = %q", a.FileName)
	}
	if got, want := e.q.kinds(b.ID), []jobs.Kind{jobs.KindExtract}; !reflect.DeepEqual(got, want) {
		t.Fatalf("jobs = %v, want %v", got, want)
	}

	twin, err := e.svc.AddArchive(ctx, b.ID, ArchiveInput{Source: bookmarks.SourceExtension, ContentType: "text/html", Body: strings.NewReader("other")})
	if err != nil {
		t.Fatal(err)
	}
	if twin.FileName == a.FileName {
		t.Fatalf("archives at the same instant share file %q", a.FileName)
	}
	if err := e.svc.DeleteArchive(ctx, twin.ID); err != nil {
		t.Fatal(err)
	}

	got, rc, err := e.svc.OpenArchive(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(rc)
	rc.Close()
	if got.ID != a.ID || string(body) != "<html>hello</html>" {
		t.Fatalf("open = %+v %q", got, body)
	}

	var ve *ValidationError
	if _, err := e.svc.AddArchive(ctx, b.ID, ArchiveInput{Source: "weird", Body: strings.NewReader("x")}); !errors.As(err, &ve) {
		t.Fatalf("err = %v, want ValidationError", err)
	}
	// "import" only labels migrated archives; new ones can't use it.
	if _, err := e.svc.AddArchive(ctx, b.ID, ArchiveInput{Source: bookmarks.SourceImport, Body: strings.NewReader("x")}); !errors.As(err, &ve) {
		t.Fatalf("import source: err = %v, want ValidationError", err)
	}
	if _, err := e.svc.AddArchive(ctx, 999, ArchiveInput{Source: bookmarks.SourceUpload, Body: strings.NewReader("x")}); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	big := strings.NewReader(strings.Repeat("x", 1<<20+1))
	if _, err := e.svc.AddArchive(ctx, b.ID, ArchiveInput{Source: bookmarks.SourceUpload, ContentType: "text/plain", Body: big}); !errors.Is(err, archive.ErrTooLarge) {
		t.Fatalf("err = %v, want ErrTooLarge", err)
	}
	list, _ := e.st.ListArchives(ctx, b.ID)
	if len(list) != 1 {
		t.Fatalf("archives = %d, want 1", len(list))
	}

	if err := e.svc.DeleteArchive(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(e.dir, "archives", filepath.FromSlash(a.FileName))); !os.IsNotExist(err) {
		t.Fatalf("archive file still exists: %v", err)
	}
	if _, _, err := e.svc.OpenArchive(ctx, a.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestDeleteRemovesFiles(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	b, _, _ := e.svc.Save(ctx, SaveInput{URL: "https://a.example"})
	a, err := e.svc.AddArchive(ctx, b.ID, ArchiveInput{Source: bookmarks.SourceUpload, ContentType: "application/pdf", Body: strings.NewReader("%PDF")})
	if err != nil {
		t.Fatal(err)
	}
	previews := filepath.Join(e.dir, "previews")
	os.MkdirAll(previews, 0o755)
	os.WriteFile(filepath.Join(previews, "p.png"), []byte("png"), 0o644)
	b.PreviewFile = "p.png"
	if err := e.st.UpdateBookmark(ctx, b); err != nil {
		t.Fatal(err)
	}

	if err := e.svc.Delete(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(e.dir, "archives", filepath.FromSlash(a.FileName))); !os.IsNotExist(err) {
		t.Fatalf("archive file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(previews, "p.png")); !os.IsNotExist(err) {
		t.Fatalf("preview file: %v", err)
	}
	if _, err := e.svc.Get(ctx, b.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
	if err := e.svc.Delete(ctx, b.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestDeleteKeepsSharedPreview(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	previews := filepath.Join(e.dir, "previews")
	os.MkdirAll(previews, 0o755)
	file := filepath.Join(previews, "shared.png")
	os.WriteFile(file, []byte("png"), 0o644)
	var ids []int64
	for _, u := range []string{"https://a.example", "https://b.example"} {
		b, _, err := e.svc.Save(ctx, SaveInput{URL: u})
		if err != nil {
			t.Fatal(err)
		}
		b.PreviewFile = "shared.png"
		if err := e.st.UpdateBookmark(ctx, b); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, b.ID)
	}
	if err := e.svc.Delete(ctx, ids[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); err != nil {
		t.Fatalf("shared preview removed while still used: %v", err)
	}
	if err := e.svc.Delete(ctx, ids[1]); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("preview not removed after last delete: %v", err)
	}
}

func TestReaderWithoutText(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	b, _, _ := e.svc.Save(ctx, SaveInput{URL: "https://a.example"})
	p, err := e.svc.Reader(ctx, b.ID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if p.Status != bookmarks.ReaderNone || p.Markdown != "" || p.NextOffset != nil || p.Bookmark.ID != b.ID {
		t.Fatalf("page = %+v", p)
	}
}

func TestReaderPaging(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	b, _, _ := e.svc.Save(ctx, SaveInput{URL: "https://a.example"})

	var sb strings.Builder
	para := strings.Repeat("é", 299) + "\n\n" // 301 runes
	for utf8.RuneCountInString(sb.String()) < 10000 {
		sb.WriteString(para)
	}
	text := []rune(sb.String())[:10000]
	md := string(text)
	if err := e.st.PutReaderText(ctx, bookmarks.ReaderText{BookmarkID: b.ID, Status: bookmarks.ReaderOK, Markdown: md, WordCount: 33}); err != nil {
		t.Fatal(err)
	}

	var pages []ReaderPage
	offset := 0
	for {
		p, err := e.svc.Reader(ctx, b.ID, offset, 4000)
		if err != nil {
			t.Fatal(err)
		}
		pages = append(pages, p)
		if p.NextOffset == nil {
			break
		}
		if len(pages) > 5 {
			t.Fatal("too many pages")
		}
		offset = *p.NextOffset
	}
	if len(pages) != 3 {
		t.Fatalf("pages = %d, want 3", len(pages))
	}
	var joined strings.Builder
	for i, p := range pages {
		joined.WriteString(p.Markdown)
		if p.TotalChars != 10000 || p.Status != bookmarks.ReaderOK || p.WordCount != 33 {
			t.Fatalf("page %d = %+v", i, p)
		}
		if n := utf8.RuneCountInString(p.Markdown); n > 4000 {
			t.Fatalf("page %d has %d runes", i, n)
		}
		if i < 2 && !strings.HasSuffix(p.Markdown, "\n\n") {
			t.Fatalf("page %d not cut at a paragraph break", i)
		}
	}
	if joined.String() != md {
		t.Fatal("pages do not concatenate to the original")
	}

	// Hard cut when the window has no paragraph break.
	if err := e.st.PutReaderText(ctx, bookmarks.ReaderText{BookmarkID: b.ID, Status: bookmarks.ReaderOK, Markdown: strings.Repeat("a", 50)}); err != nil {
		t.Fatal(err)
	}
	p, err := e.svc.Reader(ctx, b.ID, 0, 20)
	if err != nil || len(p.Markdown) != 20 || p.NextOffset == nil || *p.NextOffset != 20 {
		t.Fatalf("hard cut = %+v, %v", p, err)
	}
	p, err = e.svc.Reader(ctx, b.ID, 0, 0)
	if err != nil || len(p.Markdown) != 50 || p.NextOffset != nil {
		t.Fatalf("whole = %+v, %v", p, err)
	}
	var ve *ValidationError
	if _, err := e.svc.Reader(ctx, b.ID, 51, 0); !errors.As(err, &ve) {
		t.Fatalf("err = %v", err)
	}
	if _, err := e.svc.Reader(ctx, b.ID, -1, 0); !errors.As(err, &ve) {
		t.Fatalf("err = %v", err)
	}
	if _, err := e.svc.Reader(ctx, 999, 0, 0); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestRequestCaptureAndExtract(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	b, _, _ := e.svc.Save(ctx, SaveInput{URL: "https://a.example", Capture: CaptureNone})
	e.q.reset()
	if err := e.svc.RequestCapture(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.RequestExtract(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	if got, want := e.q.kinds(b.ID), []jobs.Kind{jobs.KindCapture, jobs.KindExtract}; !reflect.DeepEqual(got, want) {
		t.Fatalf("jobs = %v", got)
	}
	if err := e.svc.RequestCapture(ctx, 999); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestListArchives(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	if _, err := e.svc.ListArchives(ctx, 999); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	b, _, _ := e.svc.Save(ctx, SaveInput{URL: "https://example.com/l", Capture: CaptureNone})
	if _, err := e.svc.AddArchive(ctx, b.ID, ArchiveInput{Source: bookmarks.SourceUpload, ContentType: "text/html", Body: strings.NewReader("x")}); err != nil {
		t.Fatal(err)
	}
	list, err := e.svc.ListArchives(ctx, b.ID)
	if err != nil || len(list) != 1 {
		t.Fatalf("list = %v, %v", list, err)
	}
}
