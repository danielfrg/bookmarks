package service

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/danielfrg/bookmarks/internal/bookmarks"
)

func TestNetscapeRoundTrip(t *testing.T) {
	ctx := context.Background()
	src := newEnv(t)
	added := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	modified := time.Date(2025, 6, 7, 8, 9, 10, 0, time.UTC)
	save := func(in SaveInput) {
		t.Helper()
		in.Capture = CaptureNone
		if _, _, err := src.svc.Save(ctx, in); err != nil {
			t.Fatal(err)
		}
	}
	save(SaveInput{URL: "https://a.example/?q=1&r=2", Title: ptr(`A "quoted" <title> & more`), Description: ptr("desc <b> & stuff"),
		Notes: ptr("my <notes>\nline 2"), Tags: &[]string{"go", "web"}, Unread: ptr(true), DateAdded: &added, DateModified: &modified})
	save(SaveInput{URL: "https://b.example/", Title: ptr("B"), Tags: &[]string{"old"},
		DateAdded: ptr(added.Add(time.Hour)), DateModified: ptr(added.Add(2 * time.Hour))})
	save(SaveInput{URL: "https://c.example/", Title: ptr("C"), DateAdded: ptr(added.Add(2 * time.Hour)), DateModified: ptr(added.Add(2 * time.Hour))})

	var buf bytes.Buffer
	if err := src.svc.ExportNetscape(ctx, &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(buf.String(), "<!DOCTYPE NETSCAPE-Bookmark-file-1>") {
		t.Fatalf("bad header: %q", buf.String())
	}

	dst := newEnv(t)
	res, err := dst.svc.ImportNetscape(ctx, bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if res != (ImportResult{Created: 3}) {
		t.Fatalf("result = %+v", res)
	}
	if got := dst.q.kinds(1); len(got) == 0 {
		t.Fatal("import should still enqueue the metadata job")
	}
	for _, k := range dst.q.kinds(1) {
		if k == "capture" {
			t.Fatalf("import must not enqueue capture: %v", dst.q.kinds(1))
		}
	}

	list := func(e *env) []bookmarks.Bookmark {
		r, err := e.svc.Search(ctx, SearchInput{Sort: "added_asc", Limit: 10})
		if err != nil {
			t.Fatal(err)
		}
		var out []bookmarks.Bookmark
		for _, h := range r.Hits {
			out = append(out, h.Bookmark)
		}
		return out
	}
	want, got := list(src), list(dst)
	if len(got) != 3 {
		t.Fatalf("got %d bookmarks", len(got))
	}
	for i := range want {
		w, g := want[i], got[i]
		if w.URL != g.URL || w.Title != g.Title || w.Description != g.Description || w.Notes != g.Notes ||
			w.Unread != g.Unread || strings.Join(w.Tags, ",") != strings.Join(g.Tags, ",") ||
			!w.DateAdded.Equal(g.DateAdded) || !w.DateModified.Equal(g.DateModified) {
			t.Errorf("bookmark %d differs:\nwant %+v\ngot  %+v", i, w, g)
		}
	}

	// Importing again updates instead of creating.
	res, err = dst.svc.ImportNetscape(ctx, bytes.NewReader(buf.Bytes()))
	if err != nil || res != (ImportResult{Updated: 3}) {
		t.Fatalf("re-import = %+v, %v", res, err)
	}
}

func TestImportNetscapeFile(t *testing.T) {
	// A typical exported file: CRLF-ish, unclosed <DT>/<DD>/<p>, mixed case
	// attributes, millisecond timestamps and an invalid URL.
	const file = `<!DOCTYPE NETSCAPE-Bookmark-file-1>
<META HTTP-EQUIV="Content-Type" CONTENT="text/html; charset=UTF-8">
<TITLE>Bookmarks</TITLE>
<H1>Bookmarks</H1>
<DL><p>
<DT><A HREF="https://example.com/1" ADD_DATE="1616337559" LAST_MODIFIED="1616337600" PRIVATE="0" TOREAD="1" TAGS="Tag1,tag2">test title 1</A>
<DD>test description 1[bookmarks-notes]some &lt;notes&gt;[/bookmarks-notes]
<DT><A HREF="https://example.com/2" ADD_DATE="1616337559000" PRIVATE="1" TOREAD="0" TAGS="">test title 2</A>
<DT><A HREF="not a url" ADD_DATE="1616337559">bad</A>
<DD>bad desc
<DT><a href="https://example.com/4">only href</a>
</DL><p>`
	e := newEnv(t)
	res, err := e.svc.ImportNetscape(context.Background(), strings.NewReader(file))
	if err != nil {
		t.Fatal(err)
	}
	if res != (ImportResult{Created: 3, Skipped: 1}) {
		t.Fatalf("result = %+v", res)
	}
	b, err := e.svc.st.GetBookmarkByURL(context.Background(), "https://example.com/1")
	if err != nil {
		t.Fatal(err)
	}
	if b.Title != "test title 1" || b.Description != "test description 1" || b.Notes != "some <notes>" ||
		!b.Unread || strings.Join(b.Tags, ",") != "tag1,tag2" ||
		!b.DateAdded.Equal(time.Unix(1616337559, 0)) || !b.DateModified.Equal(time.Unix(1616337600, 0)) {
		t.Errorf("bookmark 1 = %+v", b)
	}
	b2, _ := e.svc.st.GetBookmarkByURL(context.Background(), "https://example.com/2")
	if !b2.DateAdded.Equal(time.Unix(1616337559, 0)) || b2.Unread {
		t.Errorf("bookmark 2 = %+v", b2)
	}
	b4, err := e.svc.st.GetBookmarkByURL(context.Background(), "https://example.com/4")
	if err != nil || b4.Description != "" {
		t.Errorf("bookmark 4 = %+v, %v", b4, err)
	}
}

func TestImportMergesIntoExisting(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	if _, _, err := e.svc.Save(ctx, SaveInput{URL: "https://m.example/", Title: ptr("T"), Description: ptr("keep me"),
		Notes: ptr("keep notes"), Tags: &[]string{"mine", "shared"}, Unread: ptr(true), Capture: CaptureNone}); err != nil {
		t.Fatal(err)
	}
	res, err := e.svc.ImportNetscape(ctx, strings.NewReader(`<DL><DT><A HREF="https://m.example/" TAGS="shared,new" TOREAD="0" ADD_DATE="1000000000" LAST_MODIFIED="1000000000"></A></DL>`))
	if err != nil || res != (ImportResult{Updated: 1}) {
		t.Fatalf("%+v %v", res, err)
	}
	b, _ := e.svc.st.GetBookmarkByURL(ctx, "https://m.example/")
	if !b.DateAdded.Equal(fixedNow) || !b.DateModified.Equal(fixedNow) {
		t.Fatalf("dates changed: added %v modified %v", b.DateAdded, b.DateModified)
	}
	if strings.Join(b.Tags, ",") != "mine,new,shared" || b.Description != "keep me" || b.Notes != "keep notes" ||
		b.Title != "T" || !b.Unread {
		t.Fatalf("merged = %+v", b)
	}
}

func TestBulkDeleteEmptyQueryRejected(t *testing.T) {
	e := newEnv(t)
	_, err := e.svc.Bulk(context.Background(), BulkInput{Action: "delete", Query: &SearchInput{}})
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %v", err)
	}
}

func TestBulkDeleteBlankOrEmptyExprRejected(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	b, _, _ := e.svc.Save(ctx, SaveInput{URL: "https://keep.example", Capture: CaptureNone})
	for _, q := range []string{"   ", "*", "#", "-", "!bogus"} {
		_, err := e.svc.Bulk(ctx, BulkInput{Action: "delete", Query: &SearchInput{Q: q}})
		var ve *ValidationError
		if !errors.As(err, &ve) {
			t.Errorf("q=%q: err = %v, want ValidationError", q, err)
		}
	}
	if _, err := e.svc.Get(ctx, b.ID); err != nil {
		t.Fatalf("bookmark deleted: %v", err)
	}
}
