package service

import (
	"context"
	"strings"

	"github.com/danielfrg/bookmarks/internal/bookmarks"
)

// Tags lists every tag with its bookmark count.
func (s *Service) Tags(ctx context.Context) ([]bookmarks.Tag, error) {
	return s.st.ListTags(ctx)
}

// RenameTag renames tag id to the normalized name, merging into an existing
// tag of that name.
func (s *Service) RenameTag(ctx context.Context, id int64, name string) error {
	names := bookmarks.NormalizeTags([]string{name})
	if len(names) != 1 {
		return invalid("name", "must be a single tag name")
	}
	return s.st.RenameTag(ctx, id, names[0])
}

// DeleteTag removes tag id from every bookmark.
func (s *Service) DeleteTag(ctx context.Context, id int64) error {
	return s.st.DeleteTag(ctx, id)
}

// Bundles lists the bundles in their display order.
func (s *Service) Bundles(ctx context.Context) ([]bookmarks.Bundle, error) {
	return s.st.ListBundles(ctx)
}

// Bundle returns one bundle.
func (s *Service) Bundle(ctx context.Context, id int64) (bookmarks.Bundle, error) {
	return s.st.GetBundle(ctx, id)
}

// SaveBundle creates b (ID 0) or replaces the editable fields of bundle b.ID.
// Tag lists are normalized; FilterUnread defaults to "off".
func (s *Service) SaveBundle(ctx context.Context, b bookmarks.Bundle) (bookmarks.Bundle, error) {
	b.Name = strings.TrimSpace(b.Name)
	if b.Name == "" {
		return bookmarks.Bundle{}, invalid("name", "is required")
	}
	if b.FilterUnread == "" {
		b.FilterUnread = "off"
	}
	switch b.FilterUnread {
	case "off", "yes", "no":
	default:
		return bookmarks.Bundle{}, invalid("filter_unread", `must be "off", "yes" or "no"`)
	}
	if _, err := parseQuery("search", b.Search); err != nil {
		return bookmarks.Bundle{}, err
	}
	b.AnyTags = bookmarks.NormalizeTags(b.AnyTags)
	b.AllTags = bookmarks.NormalizeTags(b.AllTags)
	b.ExcludedTags = bookmarks.NormalizeTags(b.ExcludedTags)
	if err := s.st.SaveBundle(ctx, &b); err != nil {
		return bookmarks.Bundle{}, err
	}
	return s.st.GetBundle(ctx, b.ID)
}

// DeleteBundle removes bundle id.
func (s *Service) DeleteBundle(ctx context.Context, id int64) error {
	return s.st.DeleteBundle(ctx, id)
}

// ReorderBundles sets the bundle order to the order of ids.
func (s *Service) ReorderBundles(ctx context.Context, ids []int64) error {
	return s.st.ReorderBundles(ctx, ids)
}
