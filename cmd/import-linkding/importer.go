package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"sort"
	"strings"
	"time"
)

// Config is the import configuration (see main.go for the flags).
type Config struct {
	LinkdingURL, LinkdingToken   string // --linkding-url, env LINKDING_TOKEN
	BookmarksURL, BookmarksToken string // --bookmarks-url, env BOOKMARKS_TOKEN
	DryRun                       bool   // read everything, write nothing, print counts
	Limit                        int    // only the first N bookmarks (0 = all)
	SkipAssets                   bool   // do not copy linkding assets
}

// Summary counts what a run did. In a dry run Archives and SavedSearches
// count what would be copied, and Created/Updated stay 0.
type Summary struct {
	Bookmarks       int // bookmarks read from linkding (within --limit)
	Created         int
	Updated         int
	Archives        int // assets copied
	ArchivesSkipped int // complete assets already present in Bookmarks
	SavedSearches   int // bundles created as saved searches
	Failures        int
}

// target is a client for the Bookmarks REST API.
type target struct {
	base  string
	token string
	c     *http.Client
}

type tBookmark struct {
	ID           int64 `json:"id"`
	ArchiveCount int   `json:"archive_count"`
}

type tArchive struct {
	Source    string    `json:"source"`
	CreatedAt time.Time `json:"created_at"`
}

type tSavedSearch struct {
	Name string `json:"name"`
}

func (t *target) newRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, t.base+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+t.token)
	return req, nil
}

// doJSON sends a JSON request (60 s timeout) and decodes the response into
// out when the status is one of ok.
func (t *target) doJSON(ctx context.Context, method, path string, in, out any, ok ...int) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return 0, err
		}
		body = bytes.NewReader(b)
	}
	req, err := t.newRequest(ctx, method, path, body)
	if err != nil {
		return 0, err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := t.c.Do(req)
	if err != nil {
		return 0, fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer res.Body.Close()
	for _, s := range ok {
		if res.StatusCode == s {
			if out == nil {
				return s, nil
			}
			if err := json.NewDecoder(res.Body).Decode(out); err != nil {
				return s, fmt.Errorf("%s %s: decode: %w", method, path, err)
			}
			return s, nil
		}
	}
	return res.StatusCode, fmt.Errorf("%s %s: %s", method, path, statusError(res))
}

type importer struct {
	cfg  Config
	lk   *linkding
	bm   *target
	log  io.Writer
	sum  Summary
	seen int // bookmarks handled, for --limit
}

// Run copies all bookmarks (including ones archived in linkding), their complete assets and the
// bundles from linkding into Bookmarks. Per-item problems are counted in
// Summary.Failures and logged; an error is returned only for setup failures
// (bad URLs, rejected tokens, unreachable servers).
func Run(ctx context.Context, cfg Config, httpc *http.Client, log io.Writer) (Summary, error) {
	lkBase, err := baseURL("linkding", cfg.LinkdingURL)
	if err != nil {
		return Summary{}, err
	}
	bmBase, err := baseURL("bookmarks", cfg.BookmarksURL)
	if err != nil {
		return Summary{}, err
	}
	if cfg.LinkdingToken == "" || cfg.BookmarksToken == "" {
		return Summary{}, errors.New("both LINKDING_TOKEN and BOOKMARKS_TOKEN are required")
	}
	im := &importer{
		cfg: cfg,
		lk:  &linkding{base: lkBase, token: cfg.LinkdingToken, c: httpc},
		bm:  &target{base: strings.TrimSuffix(bmBase.String(), "/"), token: cfg.BookmarksToken, c: httpc},
		log: log,
	}

	// Setup checks: both tokens must work before anything is written.
	var profile map[string]any
	if err := im.lk.getJSON(ctx, "/api/user/profile/", &profile); err != nil {
		return Summary{}, err
	}
	var existing []tSavedSearch
	if _, err := im.bm.doJSON(ctx, http.MethodGet, "/api/saved-searches", nil, &existing, http.StatusOK); err != nil {
		return Summary{}, fmt.Errorf("bookmarks: %w", err)
	}
	if cfg.DryRun {
		fmt.Fprintln(log, "dry run: reading only, nothing is written")
	}

	for _, list := range []string{"/api/bookmarks/", "/api/bookmarks/archived/"} {
		if cfg.Limit > 0 && im.seen >= cfg.Limit {
			break
		}
		im.bookmarkList(ctx, list)
		if ctx.Err() != nil {
			return im.sum, ctx.Err()
		}
	}

	im.bundles(ctx, existing)

	s := im.sum
	prefix := ""
	if cfg.DryRun {
		prefix = "dry run, would copy: "
	}
	fmt.Fprintf(log, "\n%sbookmarks %d (created %d, updated %d), archives %d (already present %d), saved searches %d, failures %d\n",
		prefix, s.Bookmarks, s.Created, s.Updated, s.Archives, s.ArchivesSkipped, s.SavedSearches, s.Failures)
	return s, ctx.Err()
}

// listPageSize is the page size requested from linkding. Its
// LimitOffsetPagination has no max_limit, so most instances fit in one page.
const listPageSize = 1000

// bookmarkList imports one linkding bookmark list. Both the main list and
// linkding's archived list are imported; Bookmarks has no archived state, so
// they all become ordinary bookmarks.
//
// linkding orders only by "-date_added", so rows with equal dates can repeat
// or be skipped across offset pages, the same way on every run. Repeats are
// skipped by id; if fewer bookmarks than the list's count were seen, the list
// is walked again as a single page of that size and only the missing ones are
// imported. A shortfall after that is a failure.
func (im *importer) bookmarkList(ctx context.Context, list string) {
	ids := map[int64]bool{}
	stopped := false
	visit := func(b lkBookmark) bool {
		if ids[b.ID] {
			return true
		}
		ids[b.ID] = true
		im.bookmark(ctx, b)
		im.seen++
		stopped = ctx.Err() != nil || (im.cfg.Limit > 0 && im.seen >= im.cfg.Limit)
		return !stopped
	}
	total, err := walk(ctx, im.lk, fmt.Sprintf("%s?limit=%d", list, listPageSize), visit)
	if err == nil && !stopped && len(ids) < total {
		fmt.Fprintf(im.log, "%s listed %d of %d bookmarks across pages; reading it again as one page\n", list, len(ids), total)
		_, err = walk(ctx, im.lk, fmt.Sprintf("%s?limit=%d", list, total), visit)
	}
	switch {
	case err != nil:
		im.sum.Failures++
		fmt.Fprintf(im.log, "! failed: listing linkding bookmarks: %v\n", err)
	case !stopped && len(ids) < total:
		im.sum.Failures++
		fmt.Fprintf(im.log, "! failed: %s listed %d of %d bookmarks (the list changed while reading); re-run\n", list, len(ids), total)
	}
}

func baseURL(name, raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("--%s-url: must be an http(s) base URL, got %q", name, raw)
	}
	u.Path = strings.TrimSuffix(u.Path, "/")
	u.RawPath = ""
	return u, nil
}

// bookmark imports one linkding bookmark and its assets, printing one line.
func (im *importer) bookmark(ctx context.Context, b lkBookmark) {
	im.sum.Bookmarks++
	fail := func(err error) {
		im.sum.Failures++
		fmt.Fprintf(im.log, "! failed: %s: %v\n", b.URL, err)
	}
	added, err := parseTime(b.DateAdded)
	if err != nil {
		fail(fmt.Errorf("date_added: %w", err))
		return
	}
	modified, err := parseTime(b.DateModified)
	if err != nil {
		fail(fmt.Errorf("date_modified: %w", err))
		return
	}

	var assets []lkAsset
	if !im.cfg.SkipAssets {
		assets, err = im.completeAssets(ctx, b.ID)
		if err != nil {
			fail(err)
			return
		}
	}

	if im.cfg.DryRun {
		im.sum.Archives += len(assets)
		fmt.Fprintf(im.log, "= %s (%d assets)\n", b.URL, len(assets))
		return
	}

	tags := b.TagNames
	if tags == nil {
		tags = []string{}
	}
	body := map[string]any{
		"url": b.URL, "title": b.Title, "description": b.Description, "notes": b.Notes, "tags": tags,
		"unread": b.Unread, "date_added": added, "date_modified": modified,
	}
	var saved tBookmark
	status, err := im.bm.doJSON(ctx, http.MethodPost, "/api/bookmarks?capture=none", body, &saved, http.StatusCreated, http.StatusOK)
	if err != nil {
		fail(err)
		return
	}
	verb := "~ updated"
	if status == http.StatusCreated {
		im.sum.Created++
		verb = "+ created"
	} else {
		im.sum.Updated++
	}

	copied, skipped, err := im.copyAssets(ctx, b.ID, saved, assets)
	im.sum.Archives += copied
	im.sum.ArchivesSkipped += skipped
	if err != nil {
		fail(fmt.Errorf("%s, but: %w", verb[2:], err))
		return
	}
	fmt.Fprintf(im.log, "%s: %s%s\n", verb, b.URL, archiveNote(copied, skipped))
}

func archiveNote(copied, skipped int) string {
	switch {
	case copied == 0 && skipped == 0:
		return ""
	case skipped == 0:
		return fmt.Sprintf(" (%d archives)", copied)
	}
	return fmt.Sprintf(" (%d archives, %d already present)", copied, skipped)
}

// completeAssets lists the complete assets of a linkding bookmark, oldest
// first, so the newest becomes the latest archive.
func (im *importer) completeAssets(ctx context.Context, bookmarkID int64) ([]lkAsset, error) {
	type dated struct {
		a lkAsset
		t time.Time
	}
	var list []dated
	var bad error
	_, err := walk(ctx, im.lk, fmt.Sprintf("/api/bookmarks/%d/assets/?limit=100", bookmarkID), func(a lkAsset) bool {
		if a.Status != "complete" {
			return true
		}
		t, err := parseTime(a.DateCreated)
		if err != nil {
			bad = fmt.Errorf("asset %d date_created: %w", a.ID, err)
			return false
		}
		list = append(list, dated{a, t})
		return true
	})
	if err == nil {
		err = bad
	}
	if err != nil {
		return nil, fmt.Errorf("list assets: %w", err)
	}
	sort.SliceStable(list, func(i, j int) bool {
		if !list[i].t.Equal(list[j].t) {
			return list[i].t.Before(list[j].t)
		}
		return list[i].a.ID < list[j].a.ID
	})
	out := make([]lkAsset, len(list))
	for i, d := range list {
		out[i] = d.a
	}
	return out, nil
}

func archiveSource(assetType string) string {
	if assetType == "snapshot" {
		return "import"
	}
	return "upload"
}

// archiveKey identifies a copied asset in Bookmarks: its source and creation
// time (to the second, so storage precision does not matter).
func archiveKey(source string, t time.Time) string {
	return source + "|" + t.UTC().Truncate(time.Second).Format(time.RFC3339)
}

// copyAssets uploads the assets not yet present on the target bookmark. A
// bookmark without archives gets all of them; otherwise its archive list is
// matched by source and created_at, so a re-run neither duplicates archives
// nor misses the ones a previous run failed to copy. It keeps going after a
// failed asset (the latest archive is chosen by created_at, not upload order)
// and returns the first error.
func (im *importer) copyAssets(ctx context.Context, lkID int64, saved tBookmark, assets []lkAsset) (copied, skipped int, err error) {
	if len(assets) == 0 {
		return 0, 0, nil
	}
	present := map[string]int{}
	if saved.ArchiveCount > 0 {
		var list []tArchive
		if _, err := im.bm.doJSON(ctx, http.MethodGet, fmt.Sprintf("/api/bookmarks/%d/archives", saved.ID), nil, &list, http.StatusOK); err != nil {
			return 0, 0, err
		}
		for _, a := range list {
			present[archiveKey(a.Source, a.CreatedAt)]++
		}
	}
	var first error
	for _, a := range assets {
		created, _ := parseTime(a.DateCreated) // validated by completeAssets
		key := archiveKey(archiveSource(a.AssetType), created)
		if present[key] > 0 {
			present[key]--
			skipped++
			continue
		}
		if err := im.copyAsset(ctx, lkID, saved.ID, a, created); err != nil {
			if first == nil {
				first = err
			}
			if ctx.Err() != nil {
				break
			}
			continue
		}
		copied++
	}
	return copied, skipped, first
}

var quoteEscaper = strings.NewReplacer("\\", "\\\\", `"`, "\\\"", "\r", "", "\n", "")

// copyAsset streams one linkding asset into a Bookmarks archive: the download
// body is piped into the multipart upload, never buffered whole.
func (im *importer) copyAsset(ctx context.Context, lkID, bmID int64, a lkAsset, created time.Time) error {
	dl, err := im.lk.download(ctx, lkID, a.ID)
	if err != nil {
		return err
	}

	contentType := a.ContentType
	if contentType == "" {
		contentType = dl.Header.Get("Content-Type")
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	name := a.DisplayName
	if _, params, err := mime.ParseMediaType(dl.Header.Get("Content-Disposition")); err == nil && params["filename"] != "" {
		name = params["filename"]
	}

	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	done := make(chan struct{})
	go func() {
		defer close(done)
		pw.CloseWithError(func() error {
			// source and created_at must precede the file part.
			if err := mw.WriteField("source", archiveSource(a.AssetType)); err != nil {
				return err
			}
			if err := mw.WriteField("created_at", created.UTC().Format(time.RFC3339Nano)); err != nil {
				return err
			}
			h := textproto.MIMEHeader{}
			h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, quoteEscaper.Replace(name)))
			h.Set("Content-Type", contentType)
			part, err := mw.CreatePart(h)
			if err != nil {
				return err
			}
			if _, err := io.Copy(part, dl.Body); err != nil {
				return fmt.Errorf("download asset %d: %w", a.ID, err)
			}
			return mw.Close()
		}())
	}()
	defer func() {
		// Unblock the writer goroutine whether it is stuck writing to the
		// pipe (the upload stopped reading) or reading a stalled download.
		pr.Close()
		dl.Body.Close()
		<-done
	}()

	req, err := im.bm.newRequest(ctx, http.MethodPost, fmt.Sprintf("/api/bookmarks/%d/archives", bmID), pr)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	res, err := im.bm.c.Do(req)
	if err != nil {
		return fmt.Errorf("upload asset %d: %w", a.ID, err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return fmt.Errorf("upload asset %d: %s", a.ID, statusError(res))
	}
	return nil
}

// bundles copies linkding bundles as saved searches, in linkding's order.
// Names that already exist (case-insensitive) are skipped.
func (im *importer) bundles(ctx context.Context, existing []tSavedSearch) {
	var list []lkBundle
	if _, err := walk(ctx, im.lk, "/api/bundles/?limit=100", func(b lkBundle) bool {
		list = append(list, b)
		return true
	}); err != nil {
		im.sum.Failures++
		fmt.Fprintf(im.log, "! failed: listing linkding bundles: %v\n", err)
		return
	}
	sort.SliceStable(list, func(i, j int) bool { return list[i].Order < list[j].Order })
	names := map[string]bool{}
	for _, s := range existing {
		names[strings.ToLower(strings.TrimSpace(s.Name))] = true
	}
	for _, b := range list {
		key := strings.ToLower(strings.TrimSpace(b.Name))
		if names[key] {
			fmt.Fprintf(im.log, "= saved search %q already exists\n", b.Name)
			continue
		}
		if im.cfg.DryRun {
			names[key] = true
			im.sum.SavedSearches++
			fmt.Fprintf(im.log, "= saved search %q\n", b.Name)
			continue
		}
		filter := b.FilterUnread
		if filter == "" {
			filter = "off"
		}
		body := map[string]any{
			"name": b.Name, "query": b.Search, "filter_unread": filter,
			"any_tags": splitTags(b.AnyTags), "all_tags": splitTags(b.AllTags), "excluded_tags": splitTags(b.ExcludedTags),
		}
		if _, err := im.bm.doJSON(ctx, http.MethodPost, "/api/saved-searches", body, nil, http.StatusCreated); err != nil {
			im.sum.Failures++
			fmt.Fprintf(im.log, "! failed: saved search %q: %v\n", b.Name, err)
			continue
		}
		names[key] = true
		im.sum.SavedSearches++
		fmt.Fprintf(im.log, "+ created: saved search %q\n", b.Name)
	}
}

func splitTags(s string) []string {
	f := strings.Fields(s)
	if f == nil {
		return []string{}
	}
	return f
}
