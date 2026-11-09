package service

import (
	"context"
	"errors"
	"slices"

	"github.com/danielfrg/bookmarks/internal/bookmarks"
	"github.com/danielfrg/bookmarks/internal/jobs"
	"github.com/danielfrg/bookmarks/internal/store"
)

// BulkInput selects bookmarks by IDs, or by Query when IDs is empty, and an
// action: archive, unarchive, delete, tag, untag, read, unread,
// refresh_metadata, capture or extract. Tags is used by tag and untag.
type BulkInput struct {
	IDs    []int64
	Query  *SearchInput
	Action string
	Tags   []string
}

const bulkPageSize = 500

// Bulk applies an action to many bookmarks and returns how many it affected.
// Unknown IDs are skipped.
func (s *Service) Bulk(ctx context.Context, in BulkInput) (int, error) {
	var tags []string
	var apply func(ctx context.Context, id int64) error
	edit := func(fn func(*bookmarks.Bookmark)) func(context.Context, int64) error {
		return func(ctx context.Context, id int64) error {
			b, err := s.st.GetBookmark(ctx, id)
			if err != nil {
				return err
			}
			fn(&b)
			b.DateModified = s.now()
			return s.st.UpdateBookmark(ctx, b)
		}
	}
	enqueue := func(kind jobs.Kind) func(context.Context, int64) error {
		return func(ctx context.Context, id int64) error { return s.request(ctx, kind, id) }
	}
	switch in.Action {
	case "archive":
		apply = edit(func(b *bookmarks.Bookmark) { b.Archived = true })
	case "unarchive":
		apply = edit(func(b *bookmarks.Bookmark) { b.Archived = false })
	case "read":
		apply = edit(func(b *bookmarks.Bookmark) { b.Unread = false })
	case "unread":
		apply = edit(func(b *bookmarks.Bookmark) { b.Unread = true })
	case "tag", "untag":
		if tags = bookmarks.NormalizeTags(in.Tags); len(tags) == 0 {
			return 0, invalid("tags", "are required")
		}
		if in.Action == "tag" {
			apply = edit(func(b *bookmarks.Bookmark) {
				b.Tags = bookmarks.NormalizeTags(append(b.Tags, tags...))
			})
		} else {
			apply = edit(func(b *bookmarks.Bookmark) {
				b.Tags = slices.DeleteFunc(b.Tags, func(t string) bool { return slices.Contains(tags, t) })
			})
		}
	case "delete":
		apply = s.Delete
	case "refresh_metadata":
		apply = enqueue(jobs.KindMetadata)
	case "capture":
		apply = enqueue(jobs.KindCapture)
	case "extract":
		apply = enqueue(jobs.KindExtract)
	default:
		return 0, invalid("action", "unknown action")
	}

	if in.Action == "delete" && len(in.IDs) == 0 && in.Query != nil && in.Query.Q == "" && in.Query.Unread == "" {
		return 0, invalid("query", "must not be empty for delete; send ids instead")
	}
	ids, err := s.bulkIDs(ctx, in)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, id := range ids {
		err := apply(ctx, id)
		if errors.Is(err, store.ErrNotFound) {
			continue
		}
		if err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// bulkIDs returns in.IDs, or every bookmark matching in.Query (collected
// before any change so paging is not disturbed by the action).
func (s *Service) bulkIDs(ctx context.Context, in BulkInput) ([]int64, error) {
	if len(in.IDs) > 0 {
		return in.IDs, nil
	}
	if in.Query == nil {
		return nil, invalid("ids", "is required when no query is given")
	}
	q := *in.Query
	q.Limit, q.Offset, q.Snippets = bulkPageSize, 0, false
	var ids []int64
	for {
		res, err := s.Search(ctx, q)
		if err != nil {
			return nil, err
		}
		for _, h := range res.Hits {
			ids = append(ids, h.ID)
		}
		q.Offset += len(res.Hits)
		if len(res.Hits) == 0 || q.Offset >= res.Total {
			return ids, nil
		}
	}
}
