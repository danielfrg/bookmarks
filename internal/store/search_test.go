package store

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/danielfrg/bookmarks/internal/bookmarks"
	"github.com/danielfrg/bookmarks/internal/query"
)

var searchBase = time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)

// addBM creates a bookmark; i offsets dates so ordering is deterministic.
func addBM(t *testing.T, s *SQLite, i int, url, title string, tags ...string) int64 {
	t.Helper()
	b := &bookmarks.Bookmark{URL: url, Title: title, Tags: tags,
		DateAdded: searchBase.Add(time.Duration(i) * time.Hour), DateModified: searchBase.Add(time.Duration(-i) * time.Hour)}
	if err := s.CreateBookmark(context.Background(), b); err != nil {
		t.Fatal(err)
	}
	return b.ID
}

func addArchive(t *testing.T, s *SQLite, id int64, source string) {
	t.Helper()
	if _, err := s.db.Exec(`INSERT INTO archives(bookmark_id,source,created_at) VALUES(?,?,?)`, id, source, formatTime(now())); err != nil {
		t.Fatal(err)
	}
}

func setReader(t *testing.T, s *SQLite, id int64, md string) {
	t.Helper()
	ctx := context.Background()
	if _, err := s.db.Exec(`INSERT OR REPLACE INTO reader_texts(bookmark_id,status,markdown,extracted_at) VALUES(?,?,?,?)`, id, "ok", md, formatTime(now())); err != nil {
		t.Fatal(err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.reindex(ctx, tx, id); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func parse(t *testing.T, q string) query.Expr {
	t.Helper()
	e, err := query.Parse(q)
	if err != nil {
		t.Fatalf("parse %q: %v", q, err)
	}
	return e
}

func ids(r SearchResult) []int64 {
	out := []int64{}
	for _, h := range r.Hits {
		out = append(out, h.ID)
	}
	return out
}

func search(t *testing.T, s *SQLite, q SearchQuery) SearchResult {
	t.Helper()
	r, err := s.Search(context.Background(), q)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	return r
}

func sameSet(t *testing.T, got []int64, want ...int64) {
	t.Helper()
	m := map[int64]bool{}
	for _, g := range got {
		m[g] = true
	}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for _, w := range want {
		if !m[w] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}

func TestSearchMatchesReaderText(t *testing.T) {
	s := openTest(t)
	a := addBM(t, s, 1, "https://a.test", "Plain")
	addBM(t, s, 2, "https://b.test", "Other")
	setReader(t, s, a, "The body talks about adrenaline rushes at length.")
	r := search(t, s, SearchQuery{Expr: parse(t, "adrenaline"), Snippets: true})
	if !reflect.DeepEqual(ids(r), []int64{a}) || r.Total != 1 {
		t.Fatalf("got %+v", r)
	}
	if !strings.Contains(r.Hits[0].Snippet, "**adrenaline**") {
		t.Fatalf("snippet %q", r.Hits[0].Snippet)
	}
	if r.Hits[0].ReaderStatus != bookmarks.ReaderOK {
		t.Fatalf("reader status %q", r.Hits[0].ReaderStatus)
	}
	r = search(t, s, SearchQuery{Expr: parse(t, "adrenaline")})
	if r.Hits[0].Snippet != "" {
		t.Fatal("snippet without Snippets")
	}
}

func TestSearchPrefix(t *testing.T) {
	s := openTest(t)
	a := addBM(t, s, 1, "https://a.test", "SQLite FTS5")
	addBM(t, s, 2, "https://b.test", "Other")
	sameSet(t, ids(search(t, s, SearchQuery{Expr: parse(t, "sql")})), a)
	sameSet(t, ids(search(t, s, SearchQuery{Expr: parse(t, `"sqlite fts5"`)})), a)
}

func TestSearchTagsKeywordsBooleans(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	b1 := addBM(t, s, 1, "https://1.test", "one", "speaking")
	b2 := addBM(t, s, 2, "https://2.test", "two", "speaking")
	b3 := addBM(t, s, 3, "https://3.test", "three", "a")
	b4 := addBM(t, s, 4, "https://4.test", "four", "b")
	b5 := addBM(t, s, 5, "https://5.test", "five")
	for _, id := range []int64{b1} {
		bk, _ := s.GetBookmark(ctx, id)
		bk.Unread = true
		if err := s.UpdateBookmark(ctx, bk); err != nil {
			t.Fatal(err)
		}
	}
	addArchive(t, s, b3, "server")
	addArchive(t, s, b4, "server")
	addArchive(t, s, b4, "extension")
	cases := []struct {
		q    string
		want []int64
	}{
		{"#speaking !unread", []int64{b1}},
		{"#speaking not !unread", []int64{b2}},
		{"#a or #b", []int64{b3, b4}},
		{"not #a", []int64{b1, b2, b4, b5}},
		{"!untagged", []int64{b5}},
		{"!basic", []int64{b3}},
		{"!noarchive", []int64{b1, b2, b5}},
		{"!noarchive or !basic", []int64{b1, b2, b3, b5}},
		{"!bogus", []int64{b1, b2, b3, b4, b5}},
		{`""`, []int64{b1, b2, b3, b4, b5}},
		{"(#a or #b) four", []int64{b4}},
	}
	for _, c := range cases {
		r := search(t, s, SearchQuery{Expr: parse(t, c.q), Scope: "all"})
		sameSet(t, ids(r), c.want...)
		if r.Total != len(c.want) {
			t.Errorf("%q total %d", c.q, r.Total)
		}
	}
}

func TestSearchScopeUnreadSortPaging(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	a := addBM(t, s, 1, "https://a.test", "Charlie")
	b := addBM(t, s, 2, "https://b.test", "alpha")
	c := addBM(t, s, 3, "https://c.test", "Bravo")
	d := addBM(t, s, 4, "https://d.test", "delta")
	for _, id := range []int64{b, d} {
		bk, _ := s.GetBookmark(ctx, id)
		bk.Archived = true
		bk.Unread = id == d
		bk.DateModified = searchBase.Add(time.Duration(-int(id)) * time.Hour)
		s.UpdateBookmark(ctx, bk)
	}
	sameSet(t, ids(search(t, s, SearchQuery{})), a, c)
	sameSet(t, ids(search(t, s, SearchQuery{Scope: "active"})), a, c)
	sameSet(t, ids(search(t, s, SearchQuery{Scope: "archived"})), b, d)
	sameSet(t, ids(search(t, s, SearchQuery{Scope: "all"})), a, b, c, d)
	sameSet(t, ids(search(t, s, SearchQuery{Scope: "all", Unread: "no"})), a, b, c)
	sameSet(t, ids(search(t, s, SearchQuery{Scope: "all", Unread: "yes"})), d)

	order := func(sort string) []int64 {
		return ids(search(t, s, SearchQuery{Scope: "all", Sort: sort}))
	}
	for sort, want := range map[string][]int64{
		"":              {d, c, b, a},
		"added_desc":    {d, c, b, a},
		"added_asc":     {a, b, c, d},
		"modified_desc": {a, b, c, d},
		"modified_asc":  {d, c, b, a},
		"title_asc":     {b, c, a, d},
		"title_desc":    {d, a, c, b},
		"relevance":     {d, c, b, a},
	} {
		if got := order(sort); !reflect.DeepEqual(got, want) {
			t.Errorf("sort %q: got %v want %v", sort, got, want)
		}
	}
	r := search(t, s, SearchQuery{Scope: "all", Sort: "added_asc", Limit: 2, Offset: 1})
	if r.Total != 4 || !reflect.DeepEqual(ids(r), []int64{b, c}) {
		t.Fatalf("paging %+v", ids(r))
	}
	r = search(t, s, SearchQuery{Scope: "all", Sort: "added_asc", Limit: 2, Offset: -5})
	if !reflect.DeepEqual(ids(r), []int64{a, b}) {
		t.Fatalf("neg offset %+v", ids(r))
	}
	r = search(t, s, SearchQuery{Scope: "all", Limit: 100000})
	if len(r.Hits) != 4 {
		t.Fatal("clamp")
	}
}

func TestSearchRelevance(t *testing.T) {
	s := openTest(t)
	// reader-only match is newer, so added_desc fallback would put it first.
	title := addBM(t, s, 1, "https://a.test", "Kubernetes guide")
	reader := addBM(t, s, 2, "https://b.test", "Misc")
	setReader(t, s, reader, "all about kubernetes here")
	r := search(t, s, SearchQuery{Expr: parse(t, "kubernetes"), Sort: "relevance"})
	if !reflect.DeepEqual(ids(r), []int64{title, reader}) {
		t.Fatalf("got %v", ids(r))
	}
	// Negated-only query has no rankExpr: falls back, no error.
	r = search(t, s, SearchQuery{Expr: parse(t, "not kubernetes"), Sort: "relevance", Snippets: true})
	if r.Total != 0 {
		t.Fatalf("got %v", ids(r))
	}
	r = search(t, s, SearchQuery{Expr: parse(t, "#x or kubernetes"), Sort: "relevance", Snippets: true})
	if r.Total != 2 {
		t.Fatalf("got %v", ids(r))
	}
}

func TestSearchHostileInput(t *testing.T) {
	s := openTest(t)
	addBM(t, s, 1, "https://a.test", "c++ a:b NEAR title:foo it's")
	for _, in := range []string{"c++", `"unclosed`, "a:b", "NEAR", "*", "-x", "title:foo", "'", "AND OR", `"a" NEAR/2 b`, "foo*", "^x", "a OR", "(", `\`} {
		e, err := query.Parse(in)
		if err != nil {
			var se *query.SyntaxError
			if !errors.As(err, &se) {
				t.Errorf("%q: non-syntax parse error %v", in, err)
			}
			continue
		}
		if _, err := s.Search(context.Background(), SearchQuery{Expr: e, Scope: "all", Snippets: true, Sort: "relevance"}); err != nil {
			t.Errorf("%q: %v", in, err)
		}
	}
}

func TestReindexOnWrites(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	a := addBM(t, s, 1, "https://a.test", "One", "oldname")
	b := addBM(t, s, 2, "https://b.test", "Two", "other")
	sameSet(t, ids(search(t, s, SearchQuery{Expr: parse(t, "oldname")})), a)
	tags, _ := s.ListTags(ctx)
	var oldID, otherID int64
	for _, tg := range tags {
		switch tg.Name {
		case "oldname":
			oldID = tg.ID
		case "other":
			otherID = tg.ID
		}
	}
	if err := s.RenameTag(ctx, oldID, "newname"); err != nil {
		t.Fatal(err)
	}
	sameSet(t, ids(search(t, s, SearchQuery{Expr: parse(t, "newname")})), a)
	sameSet(t, ids(search(t, s, SearchQuery{Expr: parse(t, "oldname")})))
	// merge
	if err := s.RenameTag(ctx, otherID, "newname"); err != nil {
		t.Fatal(err)
	}
	sameSet(t, ids(search(t, s, SearchQuery{Expr: parse(t, "newname")})), a, b)
	sameSet(t, ids(search(t, s, SearchQuery{Expr: parse(t, "other")})))
	// update
	bk, _ := s.GetBookmark(ctx, a)
	bk.Title = "Zebra"
	s.UpdateBookmark(ctx, bk)
	sameSet(t, ids(search(t, s, SearchQuery{Expr: parse(t, "zebra")})), a)
	sameSet(t, ids(search(t, s, SearchQuery{Expr: parse(t, "one")})))
	// delete tag
	tags, _ = s.ListTags(ctx)
	if err := s.DeleteTag(ctx, tags[0].ID); err != nil {
		t.Fatal(err)
	}
	sameSet(t, ids(search(t, s, SearchQuery{Expr: parse(t, "newname")})))
	// delete bookmark
	if err := s.DeleteBookmark(ctx, a); err != nil {
		t.Fatal(err)
	}
	var n int
	s.db.QueryRow(`SELECT COUNT(*) FROM bookmarks_fts WHERE rowid=?`, a).Scan(&n)
	if n != 0 {
		t.Fatal("fts row not deleted")
	}
}

func TestSearchHitsPopulated(t *testing.T) {
	s := openTest(t)
	a := addBM(t, s, 1, "https://a.test", "Hello", "x", "y")
	addArchive(t, s, a, "server")
	r := search(t, s, SearchQuery{Expr: parse(t, "hello")})
	want, _ := s.GetBookmark(context.Background(), a)
	if len(r.Hits) != 1 || !reflect.DeepEqual(r.Hits[0].Bookmark, want) {
		t.Fatalf("got %+v want %+v", r.Hits, want)
	}
}
