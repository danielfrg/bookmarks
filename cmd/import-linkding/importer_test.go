package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/danielfrg/bookmarks/internal/capture"
	"github.com/danielfrg/bookmarks/internal/fetch"
	"github.com/danielfrg/bookmarks/internal/httpapi"
	"github.com/danielfrg/bookmarks/internal/jobs"
	"github.com/danielfrg/bookmarks/internal/service"
	"github.com/danielfrg/bookmarks/internal/store"
)

// ---- fake linkding ----

const lkToken = "lk"

type fakeBookmark struct {
	ID           int      `json:"id"`
	URL          string   `json:"url"`
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	Notes        string   `json:"notes"`
	TagNames     []string `json:"tag_names"`
	Unread       bool     `json:"unread"`
	Shared       bool     `json:"shared"`
	IsArchived   bool     `json:"is_archived"`
	DateAdded    string   `json:"date_added"`
	DateModified string   `json:"date_modified"`
}

type fakeAsset struct {
	ID          int    `json:"id"`
	Bookmark    int    `json:"bookmark"`
	AssetType   string `json:"asset_type"`
	ContentType string `json:"content_type"`
	DisplayName string `json:"display_name"`
	Status      string `json:"status"`
	DateCreated string `json:"date_created"`

	body string // served by download
	fail bool   // download answers 500
}

type fakeLinkding struct {
	t        *testing.T
	srv      *httptest.Server
	pageSize int
	tieBug   bool                // the page at offset 2 starts one item early, like unstable ordering
	noFull   bool                // do not serve the whole list when asked for limit=<count>
	limits   map[string][]string // path -> requested limit values

	mu       sync.Mutex
	active   []fakeBookmark
	archived []fakeBookmark
	assets   map[int][]*fakeAsset
	bundles  []map[string]any
	pages    map[string]int // path -> pages served
}

func newFakeLinkding(t *testing.T) *fakeLinkding {
	f := &fakeLinkding{t: t, pageSize: 2, assets: map[int][]*fakeAsset{}, pages: map[string]int{}, limits: map[string][]string{}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeLinkding) serve(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Token "+lkToken {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"detail":"Invalid token."}`)
		return
	}
	if r.Method != http.MethodGet {
		f.t.Errorf("linkding got %s %s", r.Method, r.URL)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	p := r.URL.Path
	switch {
	case p == "/api/user/profile/":
		_, _ = io.WriteString(w, `{"theme":"auto"}`)
	case p == "/api/bookmarks/":
		f.page(w, r, toAny(f.active))
	case p == "/api/bookmarks/archived/":
		f.page(w, r, toAny(f.archived))
	case p == "/api/bundles/":
		items := make([]any, len(f.bundles))
		for i, b := range f.bundles {
			items[i] = b
		}
		f.page(w, r, items)
	case strings.HasPrefix(p, "/api/bookmarks/") && strings.Contains(p, "/assets/"):
		parts := strings.Split(strings.Trim(p, "/"), "/") // api bookmarks <id> assets [<aid> download]
		bid, _ := strconv.Atoi(parts[2])
		list := f.assets[bid]
		if len(parts) == 4 {
			items := make([]any, len(list))
			for i, a := range list {
				items[i] = a
			}
			f.page(w, r, items)
			return
		}
		aid, _ := strconv.Atoi(parts[4])
		for _, a := range list {
			if a.ID == aid && len(parts) == 6 && parts[5] == "download" {
				if a.fail {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				w.Header().Set("Content-Type", a.ContentType)
				name := a.DisplayName // linkding's download_name
				if a.AssetType == "snapshot" {
					name += ".html"
				}
				w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
				_, _ = io.WriteString(w, a.body)
				return
			}
		}
		http.NotFound(w, r)
	default:
		http.NotFound(w, r)
	}
}

func toAny(bs []fakeBookmark) []any {
	out := make([]any, len(bs))
	for i, b := range bs {
		out[i] = b
	}
	return out
}

// page serves a DRF LimitOffsetPagination page with an absolute next URL.
func (f *fakeLinkding) page(w http.ResponseWriter, r *http.Request, items []any) {
	f.pages[r.URL.Path]++
	f.limits[r.URL.Path] = append(f.limits[r.URL.Path], r.URL.Query().Get("limit"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	full := limit == len(items) && !f.noFull // a single page of exactly the count is served whole
	if !full && (limit <= 0 || limit > f.pageSize) {
		limit = f.pageSize
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	start := min(offset, len(items))
	if f.tieBug && offset == 2 && !full {
		start = 1
	}
	end := min(start+limit, len(items))
	var next any
	if offset+limit < len(items) {
		next = fmt.Sprintf("%s%s?limit=%d&offset=%d", f.srv.URL, r.URL.Path, limit, offset+limit)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"count": len(items), "next": next, "previous": nil, "results": items[start:end]})
}

func (f *fakeLinkding) add(b fakeBookmark) {
	if b.DateAdded == "" {
		b.DateAdded = "2023-01-02T03:04:05.123456Z"
		b.DateModified = "2023-02-03T04:05:06.654321Z"
	}
	if b.TagNames == nil {
		b.TagNames = []string{}
	}
	if b.IsArchived {
		f.archived = append(f.archived, b)
	} else {
		f.active = append(f.active, b)
	}
}

// ---- real bookmarks server ----

type noopQueue struct{}

func (noopQueue) Enqueue(context.Context, jobs.Kind, int64) error { return nil }

type fakeCapturer struct{}

func (fakeCapturer) Capture(context.Context, string) (capture.Result, error) {
	return capture.Result{}, nil
}

type fakeTarget struct {
	srv   *httptest.Server
	token string // a valid API token of the target

	mu     sync.Mutex
	writes []string // non-GET requests
}

func newTarget(t *testing.T) *fakeTarget {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "bookmarks.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	svc := service.New(st, fetch.NewClient(fetch.Options{AllowPrivate: true}), fakeCapturer{}, noopQueue{}, service.Options{
		DataDir: dir, MaxUploadBytes: 1 << 20, Logger: log.New(io.Discard, "", 0),
	})
	_, token, err := svc.CreateToken(context.Background(), "import")
	if err != nil {
		t.Fatal(err)
	}
	h := httpapi.NewHandler(fstest.MapFS{}, httpapi.Deps{Ping: st.Ping, Service: svc, MaxUploadBytes: 1 << 20, DataDir: dir})
	tg := &fakeTarget{token: token}
	tg.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			tg.mu.Lock()
			tg.writes = append(tg.writes, r.Method+" "+r.URL.Path)
			tg.mu.Unlock()
		}
		h.ServeHTTP(w, r)
	}))
	t.Cleanup(tg.srv.Close)
	return tg
}

func (tg *fakeTarget) get(t *testing.T, path string, v any) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, tg.srv.URL+path, nil)
	req.Header.Set("Authorization", "Bearer "+tg.token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != 200 {
		t.Fatalf("GET %s: %d %s", path, res.StatusCode, body)
	}
	if s, ok := v.(*string); ok {
		*s = string(body)
		return
	}
	if err := json.Unmarshal(body, v); err != nil {
		t.Fatalf("GET %s: %v: %s", path, err, body)
	}
}

type gotBookmark struct {
	ID           int64     `json:"id"`
	URL          string    `json:"url"`
	Title        string    `json:"title"`
	Description  string    `json:"description"`
	Notes        string    `json:"notes"`
	Tags         []string  `json:"tags"`
	Unread       bool      `json:"unread"`
	DateAdded    time.Time `json:"date_added"`
	DateModified time.Time `json:"date_modified"`
	ArchiveCount int       `json:"archive_count"`
}

type gotArchive struct {
	ID          int64     `json:"id"`
	Source      string    `json:"source"`
	ContentType string    `json:"content_type"`
	DisplayName string    `json:"display_name"`
	CreatedAt   time.Time `json:"created_at"`
}

type gotSavedSearch struct {
	Name         string   `json:"name"`
	Query        string   `json:"query"`
	AnyTags      []string `json:"any_tags"`
	AllTags      []string `json:"all_tags"`
	ExcludedTags []string `json:"excluded_tags"`
	FilterUnread string   `json:"filter_unread"`
}

func (tg *fakeTarget) bookmarks(t *testing.T) []gotBookmark {
	t.Helper()
	var res struct {
		Results []gotBookmark `json:"results"`
	}
	tg.get(t, "/api/bookmarks?limit=1000", &res)
	sort.Slice(res.Results, func(i, j int) bool { return res.Results[i].URL < res.Results[j].URL })
	return res.Results
}

func (tg *fakeTarget) byURL(t *testing.T, url string) gotBookmark {
	t.Helper()
	for _, b := range tg.bookmarks(t) {
		if b.URL == url {
			return b
		}
	}
	t.Fatalf("no bookmark %s", url)
	return gotBookmark{}
}

func (tg *fakeTarget) archives(t *testing.T, id int64) []gotArchive {
	t.Helper()
	var list []gotArchive
	tg.get(t, fmt.Sprintf("/api/bookmarks/%d/archives", id), &list)
	sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })
	return list
}

func runImport(t *testing.T, lk *fakeLinkding, tg *fakeTarget, mod ...func(*Config)) (Summary, string, error) {
	t.Helper()
	cfg := Config{LinkdingURL: lk.srv.URL, LinkdingToken: lkToken, BookmarksURL: tg.srv.URL, BookmarksToken: tg.token}
	for _, m := range mod {
		m(&cfg)
	}
	var out bytes.Buffer
	s, err := Run(context.Background(), cfg, &http.Client{}, &out)
	return s, out.String(), err
}

// ---- tests ----

func TestWalksPagesAndArchivedList(t *testing.T) {
	lk, tg := newFakeLinkding(t), newTarget(t)
	for i := 1; i <= 5; i++ {
		lk.add(fakeBookmark{ID: i, URL: fmt.Sprintf("https://example.com/%d", i), Title: fmt.Sprint("T", i)})
	}
	lk.add(fakeBookmark{ID: 6, URL: "https://example.com/archived", IsArchived: true})

	s, out, err := runImport(t, lk, tg)
	if err != nil {
		t.Fatal(err)
	}
	if s.Bookmarks != 6 || s.Created != 6 || s.Updated != 0 || s.Failures != 0 {
		t.Fatalf("summary %+v\n%s", s, out)
	}
	if lk.pages["/api/bookmarks/"] != 3 || lk.pages["/api/bookmarks/archived/"] != 1 {
		t.Fatalf("pages %v", lk.pages)
	}
	got := tg.bookmarks(t)
	if len(got) != 6 {
		t.Fatalf("got %d bookmarks", len(got))
	}
	// linkding's archived bookmarks become ordinary bookmarks.
	if b := tg.byURL(t, "https://example.com/archived"); b.ID == 0 {
		t.Fatal("bookmark from linkding's archived list not imported")
	}
	if strings.Count(out, "+ ") != 6 {
		t.Fatalf("output:\n%s", out)
	}

	if got := strings.Join(lk.limits["/api/bookmarks/"], ","); got != "1000,2,2" {
		t.Fatalf("requested limits %s", got)
	}

	// Unstable paging (equal dates) repeats one bookmark and misses another
	// at limit=1000: the repeat is skipped and the list is read again as one
	// page of exactly its count, which imports the missing one.
	lk3, tg3 := newFakeLinkding(t), newTarget(t)
	lk3.active, lk3.tieBug = lk.active, true
	s, out, err = runImport(t, lk3, tg3)
	if err != nil || s.Bookmarks != 5 || s.Created != 5 || s.Failures != 0 || len(tg3.bookmarks(t)) != 5 ||
		!strings.Contains(out, "listed 4 of 5") || lk3.limits["/api/bookmarks/"][3] != "5" {
		t.Fatalf("tie: %+v %v %v\n%s", s, err, lk3.limits, out)
	}

	// If even the single page comes up short, that is a failure.
	lk4, tg4 := newFakeLinkding(t), newTarget(t)
	lk4.active, lk4.tieBug, lk4.noFull = lk.active, true, true
	s, out, err = runImport(t, lk4, tg4)
	if err != nil || s.Created != 4 || s.Failures != 1 || !strings.Contains(out, "! failed: /api/bookmarks/ listed 4 of 5") {
		t.Fatalf("persistent tie: %+v %v\n%s", s, err, out)
	}

	// --limit stops after N bookmarks.
	lk2, tg2 := newFakeLinkding(t), newTarget(t)
	lk2.active, lk2.archived = lk.active, lk.archived
	s, _, err = runImport(t, lk2, tg2, func(c *Config) { c.Limit = 3 })
	if err != nil || s.Bookmarks != 3 || len(tg2.bookmarks(t)) != 3 {
		t.Fatalf("limit: %+v %v", s, err)
	}
}

func TestPreservesDates(t *testing.T) {
	lk, tg := newFakeLinkding(t), newTarget(t)
	lk.add(fakeBookmark{
		ID: 1, URL: "https://example.com/a", Title: "A", Description: "desc", Notes: "my notes",
		TagNames: []string{"go", "web"}, Unread: true,
		DateAdded: "2021-05-06T07:08:09.123456Z", DateModified: "2022-01-02T03:04:05+02:00",
	})
	if _, out, err := runImport(t, lk, tg); err != nil {
		t.Fatal(err, out)
	}
	b := tg.byURL(t, "https://example.com/a")
	wantAdded := time.Date(2021, 5, 6, 7, 8, 9, 123456000, time.UTC)
	wantMod := time.Date(2022, 1, 2, 1, 4, 5, 0, time.UTC)
	if !b.DateAdded.Equal(wantAdded) || !b.DateModified.Equal(wantMod) {
		t.Fatalf("dates %v %v", b.DateAdded, b.DateModified)
	}
	if b.Title != "A" || b.Description != "desc" || b.Notes != "my notes" || !b.Unread || strings.Join(b.Tags, ",") != "go,web" {
		t.Fatalf("fields %+v", b)
	}

	// A re-run (merge) keeps the linkding dates.
	s, _, err := runImport(t, lk, tg)
	if err != nil || s.Updated != 1 || s.Created != 0 {
		t.Fatalf("rerun %+v %v", s, err)
	}
	b = tg.byURL(t, "https://example.com/a")
	if !b.DateAdded.Equal(wantAdded) || !b.DateModified.Equal(wantMod) {
		t.Fatalf("rerun dates %v %v", b.DateAdded, b.DateModified)
	}
}

func addAssets(lk *fakeLinkding, bid int) {
	lk.assets[bid] = []*fakeAsset{
		{ID: 10, Bookmark: bid, AssetType: "snapshot", ContentType: "text/html", DisplayName: "HTML snapshot from 01/02/2024", Status: "complete", DateCreated: "2024-01-02T10:00:00.5Z", body: "<html><body>newer snapshot</body></html>"},
		{ID: 11, Bookmark: bid, AssetType: "upload", ContentType: "text/plain", DisplayName: "notes.txt", Status: "complete", DateCreated: "2024-01-01T10:00:00Z", body: "uploaded notes"},
		{ID: 12, Bookmark: bid, AssetType: "snapshot", ContentType: "text/html", DisplayName: "pending", Status: "pending", DateCreated: "2024-01-03T10:00:00Z", body: "x"},
		{ID: 13, Bookmark: bid, AssetType: "snapshot", ContentType: "text/html", DisplayName: "failed", Status: "failure", DateCreated: "2024-01-04T10:00:00Z", body: "x"},
	}
}

func TestCopiesCompleteAssetsOldestFirstWithCreatedAt(t *testing.T) {
	lk, tg := newFakeLinkding(t), newTarget(t)
	lk.pageSize = 1 // asset list is paginated too
	lk.add(fakeBookmark{ID: 1, URL: "https://example.com/a"})
	addAssets(lk, 1)

	s, out, err := runImport(t, lk, tg)
	if err != nil {
		t.Fatal(err)
	}
	if s.Archives != 2 || s.Failures != 0 {
		t.Fatalf("summary %+v\n%s", s, out)
	}
	b := tg.byURL(t, "https://example.com/a")
	list := tg.archives(t, b.ID)
	if len(list) != 2 {
		t.Fatalf("archives %+v", list)
	}
	// Oldest first: the upload (Jan 1) is stored before the snapshot (Jan 2).
	up, snap := list[0], list[1]
	if up.Source != "upload" || up.ContentType != "text/plain" || up.DisplayName != "notes.txt" ||
		!up.CreatedAt.Equal(time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("upload %+v", up)
	}
	if snap.Source != "import" || snap.ContentType != "text/html" ||
		!snap.CreatedAt.Equal(time.Date(2024, 1, 2, 10, 0, 0, 500000000, time.UTC)) {
		t.Fatalf("snapshot %+v", snap)
	}
	var body string
	tg.get(t, fmt.Sprintf("/archives/%d/download", snap.ID), &body)
	if body != "<html><body>newer snapshot</body></html>" {
		t.Fatalf("snapshot body %q", body)
	}

	// --skip-assets copies nothing.
	lk2, tg2 := newFakeLinkding(t), newTarget(t)
	lk2.active, lk2.assets = lk.active, lk.assets
	s, _, err = runImport(t, lk2, tg2, func(c *Config) { c.SkipAssets = true })
	if err != nil || s.Archives != 0 || len(tg2.archives(t, tg2.byURL(t, "https://example.com/a").ID)) != 0 {
		t.Fatalf("skip-assets %+v %v", s, err)
	}
}

func TestRerunDoesNotDuplicateArchives(t *testing.T) {
	lk, tg := newFakeLinkding(t), newTarget(t)
	lk.add(fakeBookmark{ID: 1, URL: "https://example.com/a"})
	lk.add(fakeBookmark{ID: 2, URL: "https://example.com/b"})
	addAssets(lk, 1)
	addAssets(lk, 2)
	lk.assets[2][0].fail = true // the newer snapshot of b fails on the first run

	s, out, err := runImport(t, lk, tg)
	if err != nil || s.Archives != 3 || s.Failures != 1 {
		t.Fatalf("first run %+v %v\n%s", s, err, out)
	}

	lk.assets[2][0].fail = false
	s, out, err = runImport(t, lk, tg)
	if err != nil || s.Created != 0 || s.Updated != 2 || s.Archives != 1 || s.ArchivesSkipped != 3 || s.Failures != 0 {
		t.Fatalf("second run %+v %v\n%s", s, err, out)
	}
	s, _, err = runImport(t, lk, tg)
	if err != nil || s.Archives != 0 || s.ArchivesSkipped != 4 {
		t.Fatalf("third run %+v %v", s, err)
	}
	for _, u := range []string{"https://example.com/a", "https://example.com/b"} {
		b := tg.byURL(t, u)
		if list := tg.archives(t, b.ID); len(list) != 2 || b.ArchiveCount != 2 {
			t.Fatalf("%s archives %+v", u, list)
		}
	}
}

func TestContinuesPastFailures(t *testing.T) {
	lk, tg := newFakeLinkding(t), newTarget(t)
	lk.add(fakeBookmark{ID: 1, URL: "https://example.com/a"})
	lk.add(fakeBookmark{ID: 2, URL: "not a url"})
	lk.add(fakeBookmark{ID: 3, URL: "https://example.com/c"})
	lk.assets[1] = []*fakeAsset{{ID: 5, Bookmark: 1, AssetType: "snapshot", ContentType: "text/html", Status: "complete", DateCreated: "2024-01-01T00:00:00Z", fail: true}}

	s, out, err := runImport(t, lk, tg)
	if err != nil {
		t.Fatal(err)
	}
	if s.Failures != 2 || s.Created != 2 || s.Bookmarks != 3 {
		t.Fatalf("summary %+v\n%s", s, out)
	}
	if strings.Count(out, "! ") != 2 || !strings.Contains(out, "not a url") {
		t.Fatalf("output:\n%s", out)
	}
	if len(tg.bookmarks(t)) != 2 {
		t.Fatal("expected a and c")
	}
}

func TestOversizedAssetFailsMidStream(t *testing.T) {
	lk, tg := newFakeLinkding(t), newTarget(t) // target MaxUploadBytes is 1 MiB
	lk.add(fakeBookmark{ID: 1, URL: "https://example.com/big"})
	lk.add(fakeBookmark{ID: 2, URL: "https://example.com/next"})
	lk.assets[1] = []*fakeAsset{
		{ID: 7, Bookmark: 1, AssetType: "upload", ContentType: "application/pdf", DisplayName: "big.pdf", Status: "complete", DateCreated: "2024-01-01T00:00:00Z", body: strings.Repeat("x", 4<<20)},
		{ID: 8, Bookmark: 1, AssetType: "upload", ContentType: "text/plain", DisplayName: "small.txt", Status: "complete", DateCreated: "2024-01-02T00:00:00Z", body: "small"},
	}
	lk.assets[2] = []*fakeAsset{{ID: 9, Bookmark: 2, AssetType: "snapshot", ContentType: "text/html", Status: "complete", DateCreated: "2024-01-01T00:00:00Z", body: "<p>ok</p>"}}

	done := make(chan struct{})
	var s Summary
	var out string
	var err error
	go func() {
		defer close(done)
		s, out, err = runImport(t, lk, tg)
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("import hung on the oversized asset")
	}
	if err != nil || s.Failures != 1 || s.Created != 2 || s.Archives != 2 {
		t.Fatalf("summary %+v %v\n%s", s, err, out)
	}
	if !strings.Contains(out, "! failed: https://example.com/big: created, but: upload asset 7") {
		t.Fatalf("output:\n%s", out)
	}
	big := tg.archives(t, tg.byURL(t, "https://example.com/big").ID)
	if len(big) != 1 || big[0].DisplayName != "small.txt" {
		t.Fatalf("big archives %+v", big)
	}
	if n := len(tg.archives(t, tg.byURL(t, "https://example.com/next").ID)); n != 1 {
		t.Fatalf("next archives %d", n)
	}
}

func TestMigratesBundlesOnce(t *testing.T) {
	lk, tg := newFakeLinkding(t), newTarget(t)
	lk.bundles = []map[string]any{
		{"id": 1, "name": "Work", "search": "golang", "any_tags": "go  web", "all_tags": "", "excluded_tags": "old", "filter_unread": "yes", "filter_shared": "yes", "order": 0},
		{"id": 3, "name": "Existing", "search": "", "any_tags": "", "all_tags": "", "excluded_tags": "", "filter_unread": "off", "filter_shared": "off", "order": 1},
		{"id": 2, "name": "Reading", "search": "", "any_tags": "", "all_tags": "read later", "excluded_tags": "", "filter_unread": "no", "filter_shared": "off", "order": 2},
	}
	// A saved search with the same name (different case) already exists.
	req, _ := http.NewRequest(http.MethodPost, tg.srv.URL+"/api/saved-searches", strings.NewReader(`{"name":"existing","query":"keep"}`))
	req.Header.Set("Authorization", "Bearer "+tg.token)
	req.Header.Set("Content-Type", "application/json")
	if res, err := http.DefaultClient.Do(req); err != nil || res.StatusCode != 201 {
		t.Fatal(err, res)
	}

	s, out, err := runImport(t, lk, tg)
	if err != nil || s.SavedSearches != 2 || s.Failures != 0 {
		t.Fatalf("summary %+v %v\n%s", s, err, out)
	}
	s, _, err = runImport(t, lk, tg)
	if err != nil || s.SavedSearches != 0 {
		t.Fatalf("rerun %+v %v", s, err)
	}
	var list []gotSavedSearch
	tg.get(t, "/api/saved-searches", &list)
	if len(list) != 3 {
		t.Fatalf("saved searches %+v", list)
	}
	if list[0].Name != "existing" || list[0].Query != "keep" {
		t.Fatalf("existing changed %+v", list[0])
	}
	w, r := list[1], list[2]
	if w.Name != "Work" || w.Query != "golang" || strings.Join(w.AnyTags, ",") != "go,web" || len(w.AllTags) != 0 ||
		strings.Join(w.ExcludedTags, ",") != "old" || w.FilterUnread != "yes" {
		t.Fatalf("work %+v", w)
	}
	if r.Name != "Reading" || strings.Join(r.AllTags, ",") != "later,read" || r.FilterUnread != "no" {
		t.Fatalf("reading %+v", r)
	}
}

func TestDryRunWritesNothing(t *testing.T) {
	lk, tg := newFakeLinkding(t), newTarget(t)
	for i := 1; i <= 3; i++ {
		lk.add(fakeBookmark{ID: i, URL: fmt.Sprintf("https://example.com/%d", i)})
	}
	addAssets(lk, 1)
	lk.bundles = []map[string]any{{"id": 1, "name": "Work", "search": "", "any_tags": "", "all_tags": "", "excluded_tags": "", "filter_unread": "off", "order": 0}}

	s, out, err := runImport(t, lk, tg, func(c *Config) { c.DryRun = true })
	if err != nil {
		t.Fatal(err)
	}
	if s.Bookmarks != 3 || s.Created != 0 || s.Updated != 0 || s.Archives != 2 || s.SavedSearches != 1 || s.Failures != 0 {
		t.Fatalf("summary %+v\n%s", s, out)
	}
	if len(tg.writes) != 0 {
		t.Fatalf("writes %v", tg.writes)
	}
	if len(tg.bookmarks(t)) != 0 {
		t.Fatal("bookmarks written")
	}
	if !strings.Contains(out, "dry run") {
		t.Fatalf("output:\n%s", out)
	}
}

func TestSetupErrors(t *testing.T) {
	lk, tg := newFakeLinkding(t), newTarget(t)
	if _, _, err := runImport(t, lk, tg, func(c *Config) { c.LinkdingToken = "bad" }); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("bad linkding token: %v", err)
	}
	if _, _, err := runImport(t, lk, tg, func(c *Config) { c.BookmarksToken = "bad" }); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("bad bookmarks token: %v", err)
	}
	if _, _, err := runImport(t, lk, tg, func(c *Config) { c.LinkdingURL = "ftp://x" }); err == nil {
		t.Fatal("bad url accepted")
	}
	if len(tg.writes) != 0 {
		t.Fatalf("writes %v", tg.writes)
	}
}
