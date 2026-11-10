package mcp

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/danielfrg/bookmarks/internal/bookmarks"
	"github.com/danielfrg/bookmarks/internal/store"
)

func TestInstructionsPresent(t *testing.T) {
	e := newEnv(t)
	s := e.session(t)
	got := s.InitializeResult().Instructions
	if !strings.HasPrefix(got, "Bookmarks is Daniel's personal bookmark library") || !strings.Contains(got, "Use add_bookmark only when asked to save a link.") {
		t.Fatalf("instructions: %q", got)
	}
	tools, err := s.ListTools(context.Background(), nil)
	if err != nil || len(tools.Tools) != 4 {
		t.Fatalf("%v %v", tools, err)
	}
}

func TestSearchBookmarksTool(t *testing.T) {
	e := newEnv(t)
	e.seed(t, "https://a.example/", "Sourdough", []string{"food"}, "Feed the starter with flour and water every day.")
	e.seed(t, "https://b.example/", "Other", nil, "nothing relevant")
	s := e.session(t)
	out, _ := call(t, s, "search_bookmarks", map[string]any{"query": "starter"})
	res := out["results"].([]any)
	if len(res) != 1 || out["total"].(float64) != 1 || out["next_offset"] != nil {
		t.Fatalf("%v", out)
	}
	r := res[0].(map[string]any)
	if r["title"] != "Sourdough" || !strings.Contains(r["snippet"].(string), "starter") || r["reader_status"] != "ok" || r["url"] != "https://a.example/" {
		t.Fatalf("%v", r)
	}
	if tags := r["tags"].([]any); len(tags) != 1 || tags[0] != "food" {
		t.Fatalf("%v", r)
	}

	// Paging.
	for i := 0; i < 3; i++ {
		e.seed(t, "https://p"+string(rune('a'+i))+".example/", "Page", []string{"paged"}, "")
	}
	out, _ = call(t, s, "search_bookmarks", map[string]any{"query": "#paged", "limit": 2})
	if len(out["results"].([]any)) != 2 || out["total"].(float64) != 3 || out["next_offset"].(float64) != 2 {
		t.Fatalf("%v", out)
	}
	out, _ = call(t, s, "search_bookmarks", map[string]any{"query": "#paged", "limit": 2, "offset": 2})
	if len(out["results"].([]any)) != 1 || out["next_offset"] != nil {
		t.Fatalf("%v", out)
	}
}

func TestSearchBookmarksValidation(t *testing.T) {
	e := newEnv(t)
	s := e.session(t)
	_, res := call(t, s, "search_bookmarks", map[string]any{"query": "x", "limit": 51})
	if !res.IsError || !strings.Contains(errText(res), "limit") {
		t.Fatalf("%v", errText(res))
	}
	_, res = call(t, s, "search_bookmarks", map[string]any{"query": "x", "offset": -1})
	if !res.IsError || !strings.Contains(errText(res), "offset") {
		t.Fatalf("%v", errText(res))
	}
}

func TestReadBookmarkPaging(t *testing.T) {
	e := newEnv(t)
	text := strings.Repeat("alpha beta gamma. ", 20) + "\n\n" + strings.Repeat("delta epsilon. ", 20)
	id := e.seed(t, "https://a.example/", "Long", []string{"x"}, text)
	s := e.session(t)
	out, _ := call(t, s, "read_bookmark", map[string]any{"id": id, "max_chars": 400})
	rt := out["reader_text"].(map[string]any)
	if out["reader_status"] != "ok" || out["title"] != "Long" || out["url"] != "https://a.example/" {
		t.Fatalf("%v", out)
	}
	next := rt["next_offset"]
	if next == nil || rt["offset"].(float64) != 0 || rt["total_chars"].(float64) != float64(len([]rune(text))) {
		t.Fatalf("%v", rt)
	}
	got := rt["markdown"].(string)
	out, _ = call(t, s, "read_bookmark", map[string]any{"id": id, "offset": next, "max_chars": 8000})
	rt = out["reader_text"].(map[string]any)
	if rt["next_offset"] != nil || got+rt["markdown"].(string) != text {
		t.Fatalf("%v", rt)
	}
	_, res := call(t, s, "read_bookmark", map[string]any{"id": id, "max_chars": 30001})
	if !res.IsError {
		t.Fatal("max_chars above 30000 accepted")
	}
	_, res = call(t, s, "read_bookmark", map[string]any{"id": 9999})
	if !res.IsError || errText(res) != "Bookmark not found" {
		t.Fatalf("%q", errText(res))
	}
}

func TestReadBookmarkWithoutReaderText(t *testing.T) {
	e := newEnv(t)
	id := e.seed(t, "https://a.example/", "Bare", nil, "")
	s := e.session(t)
	_, res := call(t, s, "read_bookmark", map[string]any{"id": id})
	if res.IsError {
		t.Fatal(errText(res))
	}
	raw, _ := res.StructuredContent.(map[string]any)
	if v, present := raw["reader_text"]; !present || v != nil || raw["reader_status"] != "none" {
		t.Fatalf("%v", raw)
	}
	if _, present := raw["reader_error"]; present {
		t.Fatalf("%v", raw)
	}
}

func TestListTagsTool(t *testing.T) {
	e := newEnv(t)
	e.seed(t, "https://a.example/", "A", []string{"go", "web"}, "")
	e.seed(t, "https://b.example/", "B", []string{"go"}, "")
	out, _ := call(t, e.session(t), "list_tags", nil)
	tags := out["tags"].([]any)
	counts := map[string]float64{}
	for _, v := range tags {
		m := v.(map[string]any)
		counts[m["name"].(string)] = m["count"].(float64)
	}
	if len(counts) != 2 || counts["go"] != 2 || counts["web"] != 1 {
		t.Fatalf("%v", out)
	}
}

func TestAddBookmarkTool(t *testing.T) {
	e := newEnv(t)
	s := e.session(t)
	args := map[string]any{"url": "https://new.example/x", "title": "New", "tags": []string{"Go", "web"}, "notes": "n"}
	out, _ := call(t, s, "add_bookmark", args)
	if out["created"] != true || out["url"] != "https://new.example/x" || out["id"].(float64) < 1 {
		t.Fatalf("%v", out)
	}
	args["tags"] = []string{"more"}
	args["title"] = "Other title"
	args["notes"] = "other notes"
	args["unread"] = true
	out2, _ := call(t, s, "add_bookmark", args)
	if out2["created"] != false || out2["id"] != out["id"] {
		t.Fatalf("%v", out2)
	}
	b, err := e.svc.Get(context.Background(), int64(out["id"].(float64)))
	if err != nil || b.Title != "New" || b.Notes != "n" || b.Unread || !reflect.DeepEqual(b.Tags, []string{"go", "more", "web"}) {
		t.Fatalf("%+v %v", b, err)
	}
	_, res := call(t, s, "add_bookmark", map[string]any{"url": "ftp://nope"})
	if !res.IsError || !strings.Contains(errText(res), "url") {
		t.Fatalf("%q", errText(res))
	}
}

func TestReadBookmarkFailedReader(t *testing.T) {
	e := newEnv(t)
	id := e.seed(t, "https://a.example/", "Broken", nil, "")
	err := e.st.PutReaderText(context.Background(), bookmarks.ReaderText{BookmarkID: id, Status: bookmarks.ReaderFailed, Error: "no readable content"})
	if err != nil {
		t.Fatal(err)
	}
	out, _ := call(t, e.session(t), "read_bookmark", map[string]any{"id": id})
	if v, present := out["reader_text"]; !present || v != nil || out["reader_status"] != "failed" || out["reader_error"] != "no readable content" {
		t.Fatalf("%v", out)
	}
}

func TestSafeError(t *testing.T) {
	for _, c := range []struct {
		in   error
		want string
	}{
		{context.Canceled, "Request canceled"},
		{context.DeadlineExceeded, "Request canceled"},
		{errors.New("open /srv/data/bookmarks.db: SQL logic error"), "Bookmarks unavailable; try again"},
		{store.ErrNotFound, "Bookmark not found"},
	} {
		if got := safeError(c.in); got == nil || got.Error() != c.want {
			t.Errorf("%v: %v", c.in, got)
		}
	}
	if safeError(nil) != nil {
		t.Fatal("nil")
	}
}
