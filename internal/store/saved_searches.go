package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/danielfrg/bookmarks/internal/bookmarks"
)

const selectSavedSearch = `SELECT id,name,query,any_tags,all_tags,excluded_tags,filter_unread,sort_order,date_created,date_modified FROM saved_searches`

func scanSavedSearch(row scanner) (bookmarks.SavedSearch, error) {
	var b bookmarks.SavedSearch
	var anyTags, allTags, excluded, created, modified string
	err := row.Scan(&b.ID, &b.Name, &b.Query, &anyTags, &allTags, &excluded, &b.FilterUnread, &b.Order, &created, &modified)
	if errors.Is(err, sql.ErrNoRows) {
		return b, ErrNotFound
	}
	if err != nil {
		return b, err
	}
	b.AnyTags, b.AllTags, b.ExcludedTags = splitTags(anyTags), splitTags(allTags), splitTags(excluded)
	if b.DateCreated, err = parseTime(created); err != nil {
		return b, err
	}
	b.DateModified, err = parseTime(modified)
	return b, err
}

func (s *SQLite) ListSavedSearches(ctx context.Context) ([]bookmarks.SavedSearch, error) {
	rows, err := s.db.QueryContext(ctx, selectSavedSearch+" ORDER BY sort_order,id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []bookmarks.SavedSearch{}
	for rows.Next() {
		b, err := scanSavedSearch(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, b)
	}
	return list, rows.Err()
}

func (s *SQLite) GetSavedSearch(ctx context.Context, id int64) (bookmarks.SavedSearch, error) {
	return scanSavedSearch(s.db.QueryRowContext(ctx, selectSavedSearch+" WHERE id=?", id))
}

// SaveSavedSearch inserts when b.ID is 0 (Order becomes max+1), otherwise updates.
func (s *SQLite) SaveSavedSearch(ctx context.Context, b *bookmarks.SavedSearch) error {
	ts := now()
	if b.ID == 0 {
		b.DateCreated, b.DateModified = ts, ts
		res, err := s.db.ExecContext(ctx, `INSERT INTO saved_searches(name,query,any_tags,all_tags,excluded_tags,filter_unread,sort_order,date_created,date_modified)
 VALUES(?,?,?,?,?,?,(SELECT COALESCE(MAX(sort_order),0)+1 FROM saved_searches),?,?)`,
			b.Name, b.Query, joinTags(b.AnyTags), joinTags(b.AllTags), joinTags(b.ExcludedTags), b.FilterUnread, formatTime(ts), formatTime(ts))
		if err != nil {
			return err
		}
		if b.ID, err = res.LastInsertId(); err != nil {
			return err
		}
		return s.db.QueryRowContext(ctx, `SELECT sort_order FROM saved_searches WHERE id=?`, b.ID).Scan(&b.Order)
	}
	b.DateModified = ts
	res, err := s.db.ExecContext(ctx, `UPDATE saved_searches SET name=?,query=?,any_tags=?,all_tags=?,excluded_tags=?,filter_unread=?,date_modified=? WHERE id=?`,
		b.Name, b.Query, joinTags(b.AnyTags), joinTags(b.AllTags), joinTags(b.ExcludedTags), b.FilterUnread, formatTime(ts), b.ID)
	return affected(res, err)
}

func (s *SQLite) DeleteSavedSearch(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM saved_searches WHERE id=?`, id)
	return affected(res, err)
}

// ReorderSavedSearches sets the order of the listed saved searches to their 1-based position.
func (s *SQLite) ReorderSavedSearches(ctx context.Context, ids []int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for i, id := range ids {
		res, err := tx.ExecContext(ctx, `UPDATE saved_searches SET sort_order=? WHERE id=?`, i+1, id)
		if err := affected(res, err); err != nil {
			return err
		}
	}
	return tx.Commit()
}
