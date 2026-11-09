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

// SavedSearches lists the saved searches in their display order.
func (s *Service) SavedSearches(ctx context.Context) ([]bookmarks.SavedSearch, error) {
	return s.st.ListSavedSearches(ctx)
}

// SavedSearch returns one saved search.
func (s *Service) SavedSearch(ctx context.Context, id int64) (bookmarks.SavedSearch, error) {
	return s.st.GetSavedSearch(ctx, id)
}

// SaveSavedSearch creates b (ID 0) or replaces the editable fields of saved search b.ID.
// Tag lists are normalized; FilterUnread defaults to "off".
func (s *Service) SaveSavedSearch(ctx context.Context, b bookmarks.SavedSearch) (bookmarks.SavedSearch, error) {
	b.Name = strings.TrimSpace(b.Name)
	if b.Name == "" {
		return bookmarks.SavedSearch{}, invalid("name", "is required")
	}
	if b.FilterUnread == "" {
		b.FilterUnread = "off"
	}
	switch b.FilterUnread {
	case "off", "yes", "no":
	default:
		return bookmarks.SavedSearch{}, invalid("filter_unread", `must be "off", "yes" or "no"`)
	}
	if _, err := parseQuery("query", b.Query); err != nil {
		return bookmarks.SavedSearch{}, err
	}
	b.AnyTags = bookmarks.NormalizeTags(b.AnyTags)
	b.AllTags = bookmarks.NormalizeTags(b.AllTags)
	b.ExcludedTags = bookmarks.NormalizeTags(b.ExcludedTags)
	if err := s.st.SaveSavedSearch(ctx, &b); err != nil {
		return bookmarks.SavedSearch{}, err
	}
	return s.st.GetSavedSearch(ctx, b.ID)
}

// DeleteSavedSearch removes saved search id.
func (s *Service) DeleteSavedSearch(ctx context.Context, id int64) error {
	return s.st.DeleteSavedSearch(ctx, id)
}

// ReorderSavedSearches sets the saved search order to the order of ids.
func (s *Service) ReorderSavedSearches(ctx context.Context, ids []int64) error {
	return s.st.ReorderSavedSearches(ctx, ids)
}
