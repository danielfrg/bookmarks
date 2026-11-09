package service

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/danielfrg/bookmarks/internal/bookmarks"
	"github.com/danielfrg/bookmarks/internal/fetch"
	"github.com/danielfrg/bookmarks/internal/jobs"
	"github.com/danielfrg/bookmarks/internal/query"
	"github.com/danielfrg/bookmarks/internal/store"
)

// CaptureMode selects whether the server captures a page on save.
type CaptureMode string

const (
	CaptureServer CaptureMode = "server" // default
	CaptureNone   CaptureMode = "none"
)

// SaveInput holds bookmark fields; nil pointers mean "not provided".
type SaveInput struct {
	URL                       string
	Title, Description, Notes *string
	Tags                      *[]string
	Unread, Archived          *bool
	DateAdded, DateModified   *time.Time
	Capture                   CaptureMode
}

// MaxQueryLength is the longest accepted search query, in runes.
const MaxQueryLength = 1024

func validateURL(raw string) (string, error) {
	u, err := bookmarks.ValidateURL(raw)
	if err != nil {
		msg := strings.TrimPrefix(err.Error(), bookmarks.ErrInvalidURL.Error()+"\n")
		msg = strings.TrimPrefix(msg, "url ") // the field already names it
		return "", invalid("url", msg)
	}
	return u, nil
}

func captureMode(m CaptureMode) (CaptureMode, error) {
	switch m {
	case "", CaptureServer:
		return CaptureServer, nil
	case CaptureNone:
		return CaptureNone, nil
	}
	return "", invalid("capture", `must be "server" or "none"`)
}

// apply copies the provided fields of in onto b. Tags are normalized.
func apply(b *bookmarks.Bookmark, in SaveInput) {
	if in.Title != nil {
		b.Title = *in.Title
	}
	if in.Description != nil {
		b.Description = *in.Description
	}
	if in.Notes != nil {
		b.Notes = *in.Notes
	}
	if in.Tags != nil {
		b.Tags = bookmarks.NormalizeTags(*in.Tags)
	}
	if in.Unread != nil {
		b.Unread = *in.Unread
	}
	if in.Archived != nil {
		b.Archived = *in.Archived
	}
	if in.DateAdded != nil {
		b.DateAdded = in.DateAdded.UTC()
	}
}

// Save creates a bookmark, or updates the existing one with the same
// normalized URL (only the provided fields change).
func (s *Service) Save(ctx context.Context, in SaveInput) (bookmarks.Bookmark, bool, error) {
	u, err := validateURL(in.URL)
	if err != nil {
		return bookmarks.Bookmark{}, false, err
	}
	mode, err := captureMode(in.Capture)
	if err != nil {
		return bookmarks.Bookmark{}, false, err
	}

	existing, err := s.st.GetBookmarkByURL(ctx, u)
	switch {
	case err == nil:
		apply(&existing, in)
		existing.DateModified = s.now()
		if in.DateModified != nil {
			existing.DateModified = in.DateModified.UTC()
		}
		if err := s.st.UpdateBookmark(ctx, existing); err != nil {
			return bookmarks.Bookmark{}, false, err
		}
		if mode == CaptureServer && existing.ArchiveCount == 0 {
			s.enqueue(ctx, jobs.KindCapture, existing.ID)
		}
		b, err := s.st.GetBookmark(ctx, existing.ID)
		return b, false, err
	case !errors.Is(err, store.ErrNotFound):
		return bookmarks.Bookmark{}, false, err
	}

	settings, err := s.st.GetSettings(ctx)
	if err != nil {
		return bookmarks.Bookmark{}, false, err
	}
	b := bookmarks.Bookmark{URL: u, Unread: settings.DefaultUnread, DateAdded: s.now()}
	apply(&b, in)
	var tags []string
	if in.Tags != nil {
		tags = *in.Tags
	}
	b.Tags = bookmarks.NormalizeTags(append(tags, bookmarks.AutoTags(settings.AutoTagRules, u)...))
	b.DateModified = b.DateAdded
	if in.DateModified != nil {
		b.DateModified = in.DateModified.UTC()
	}
	if err := s.st.CreateBookmark(ctx, &b); err != nil {
		return bookmarks.Bookmark{}, false, err
	}
	s.enqueue(ctx, jobs.KindMetadata, b.ID)
	if mode != CaptureNone {
		s.enqueue(ctx, jobs.KindCapture, b.ID)
	}
	if settings.WaybackEnabled {
		s.enqueue(ctx, jobs.KindWayback, b.ID)
	}
	b, err = s.st.GetBookmark(ctx, b.ID)
	return b, true, err
}

// Update changes the provided fields of bookmark id (PATCH semantics). A new
// URL is validated; a clash with another bookmark returns store.ErrConflict.
func (s *Service) Update(ctx context.Context, id int64, in SaveInput) (bookmarks.Bookmark, error) {
	b, err := s.st.GetBookmark(ctx, id)
	if err != nil {
		return bookmarks.Bookmark{}, err
	}
	if in.URL != "" {
		u, err := validateURL(in.URL)
		if err != nil {
			return bookmarks.Bookmark{}, err
		}
		b.URL = u
	}
	apply(&b, in)
	b.DateModified = s.now()
	if in.DateModified != nil {
		b.DateModified = in.DateModified.UTC()
	}
	if err := s.st.UpdateBookmark(ctx, b); err != nil {
		return bookmarks.Bookmark{}, err
	}
	return s.st.GetBookmark(ctx, id)
}

// Get returns one bookmark.
func (s *Service) Get(ctx context.Context, id int64) (bookmarks.Bookmark, error) {
	return s.st.GetBookmark(ctx, id)
}

// FindByURL returns the bookmark with the same normalized URL, or
// store.ErrNotFound.
func (s *Service) FindByURL(ctx context.Context, rawURL string) (bookmarks.Bookmark, error) {
	u, err := validateURL(rawURL)
	if err != nil {
		return bookmarks.Bookmark{}, err
	}
	return s.st.GetBookmarkByURL(ctx, u)
}

// Delete removes a bookmark with its archive files, and its preview image when
// no other bookmark uses it. The favicon is kept: it is shared per host.
func (s *Service) Delete(ctx context.Context, id int64) error {
	b, err := s.st.GetBookmark(ctx, id)
	if err != nil {
		return err
	}
	archives, err := s.st.ListArchives(ctx, id)
	if err != nil {
		return err
	}
	if err := s.st.DeleteBookmark(ctx, id); err != nil {
		return err
	}
	for _, a := range archives {
		if err := s.files.Remove(a.FileName); err != nil {
			s.log.Printf("service: remove archive file of bookmark %d: %v", id, err)
		}
	}
	s.removePreviewIfUnused(ctx, b.PreviewFile)
	return nil
}

// SearchInput describes a bookmark search. BundleID 0 means no bundle.
type SearchInput struct {
	Q, Scope, Unread, Sort string
	BundleID               int64
	Limit, Offset          int
	Snippets               bool
}

var validSorts = map[string]bool{
	"": true, "added_desc": true, "added_asc": true, "modified_desc": true, "modified_asc": true,
	"title_asc": true, "title_desc": true, "relevance": true,
}

func parseQuery(field, q string) (query.Expr, error) {
	if utf8.RuneCountInString(q) > MaxQueryLength {
		return nil, invalid(field, "is too long")
	}
	e, err := query.Parse(q)
	var se *query.SyntaxError
	if errors.As(err, &se) {
		return nil, invalid(field, se.Error())
	}
	return e, err
}

// Search parses in.Q, combines it with the bundle's filters and runs it.
func (s *Service) Search(ctx context.Context, in SearchInput) (store.SearchResult, error) {
	switch in.Scope {
	case "", "active", "archived", "all":
	default:
		return store.SearchResult{}, invalid("scope", `must be "active", "archived" or "all"`)
	}
	switch in.Unread {
	case "", "yes", "no":
	default:
		return store.SearchResult{}, invalid("unread", `must be "yes" or "no"`)
	}
	if !validSorts[in.Sort] {
		return store.SearchResult{}, invalid("sort", "unknown sort order")
	}
	expr, err := parseQuery("q", in.Q)
	if err != nil {
		return store.SearchResult{}, err
	}
	if in.BundleID != 0 {
		b, err := s.st.GetBundle(ctx, in.BundleID)
		if err != nil {
			return store.SearchResult{}, err
		}
		be, err := bundleExpr(b)
		if err != nil {
			return store.SearchResult{}, err
		}
		expr = query.AndAll(expr, be)
	}
	return s.st.Search(ctx, store.SearchQuery{
		Expr: expr, Scope: in.Scope, Unread: in.Unread, Sort: in.Sort,
		Limit: in.Limit, Offset: in.Offset, Snippets: in.Snippets,
	})
}

// bundleExpr is the bundle's search AND (any of AnyTags) AND each AllTags AND
// NOT each ExcludedTags AND its unread filter.
func bundleExpr(b bookmarks.Bundle) (query.Expr, error) {
	search, err := parseQuery("search", b.Search)
	if err != nil {
		return nil, err
	}
	var anyTag query.Expr
	for _, t := range b.AnyTags {
		if anyTag == nil {
			anyTag = query.Tag{Name: t}
		} else {
			anyTag = query.Or{L: anyTag, R: query.Tag{Name: t}}
		}
	}
	parts := []query.Expr{search, anyTag}
	for _, t := range b.AllTags {
		parts = append(parts, query.Tag{Name: t})
	}
	for _, t := range b.ExcludedTags {
		parts = append(parts, query.Not{X: query.Tag{Name: t}})
	}
	switch b.FilterUnread {
	case "yes":
		parts = append(parts, query.Keyword{Name: "unread"})
	case "no":
		parts = append(parts, query.Not{X: query.Keyword{Name: "unread"}})
	}
	return query.AndAll(parts...), nil
}

// CheckResult describes a URL before it is saved.
type CheckResult struct {
	Bookmark         *bookmarks.Bookmark
	BasicCaptureOnly bool
	Metadata         fetch.Metadata
	AutoTags         []string
}

// Check returns the existing bookmark for rawURL (if any), the page metadata
// (cached; empty when the fetch fails) and the auto tags that would apply.
func (s *Service) Check(ctx context.Context, rawURL string) (CheckResult, error) {
	u, err := validateURL(rawURL)
	if err != nil {
		return CheckResult{}, err
	}
	var res CheckResult
	b, err := s.st.GetBookmarkByURL(ctx, u)
	switch {
	case err == nil:
		res.Bookmark = &b
		res.BasicCaptureOnly = b.BasicCaptureOnly
	case !errors.Is(err, store.ErrNotFound):
		return CheckResult{}, err
	}
	settings, err := s.st.GetSettings(ctx)
	if err != nil {
		return CheckResult{}, err
	}
	res.AutoTags = bookmarks.AutoTags(settings.AutoTagRules, u)

	if md, ok := s.meta.get(u); ok {
		res.Metadata = md
	} else if md, err := s.fc.Metadata(ctx, u); err == nil {
		s.meta.put(u, md)
		res.Metadata = md
	}
	return res, nil
}
