package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/danielfrg/bookmarks/internal/bookmarks"
)

const selectBundle = `SELECT id,name,search,any_tags,all_tags,excluded_tags,filter_unread,sort_order,date_created,date_modified FROM bundles`

func scanBundle(row scanner) (bookmarks.Bundle, error) {
	var b bookmarks.Bundle
	var anyTags, allTags, excluded, created, modified string
	err := row.Scan(&b.ID, &b.Name, &b.Search, &anyTags, &allTags, &excluded, &b.FilterUnread, &b.Order, &created, &modified)
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

func (s *SQLite) ListBundles(ctx context.Context) ([]bookmarks.Bundle, error) {
	rows, err := s.db.QueryContext(ctx, selectBundle+" ORDER BY sort_order,id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []bookmarks.Bundle{}
	for rows.Next() {
		b, err := scanBundle(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, b)
	}
	return list, rows.Err()
}

func (s *SQLite) GetBundle(ctx context.Context, id int64) (bookmarks.Bundle, error) {
	return scanBundle(s.db.QueryRowContext(ctx, selectBundle+" WHERE id=?", id))
}

// SaveBundle inserts when b.ID is 0 (Order becomes max+1), otherwise updates.
func (s *SQLite) SaveBundle(ctx context.Context, b *bookmarks.Bundle) error {
	ts := now()
	if b.ID == 0 {
		b.DateCreated, b.DateModified = ts, ts
		res, err := s.db.ExecContext(ctx, `INSERT INTO bundles(name,search,any_tags,all_tags,excluded_tags,filter_unread,sort_order,date_created,date_modified)
 VALUES(?,?,?,?,?,?,(SELECT COALESCE(MAX(sort_order),0)+1 FROM bundles),?,?)`,
			b.Name, b.Search, joinTags(b.AnyTags), joinTags(b.AllTags), joinTags(b.ExcludedTags), b.FilterUnread, formatTime(ts), formatTime(ts))
		if err != nil {
			return err
		}
		if b.ID, err = res.LastInsertId(); err != nil {
			return err
		}
		return s.db.QueryRowContext(ctx, `SELECT sort_order FROM bundles WHERE id=?`, b.ID).Scan(&b.Order)
	}
	b.DateModified = ts
	res, err := s.db.ExecContext(ctx, `UPDATE bundles SET name=?,search=?,any_tags=?,all_tags=?,excluded_tags=?,filter_unread=?,date_modified=? WHERE id=?`,
		b.Name, b.Search, joinTags(b.AnyTags), joinTags(b.AllTags), joinTags(b.ExcludedTags), b.FilterUnread, formatTime(ts), b.ID)
	return affected(res, err)
}

func (s *SQLite) DeleteBundle(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM bundles WHERE id=?`, id)
	return affected(res, err)
}

// ReorderBundles sets the order of the listed bundles to their 1-based position.
func (s *SQLite) ReorderBundles(ctx context.Context, ids []int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for i, id := range ids {
		res, err := tx.ExecContext(ctx, `UPDATE bundles SET sort_order=? WHERE id=?`, i+1, id)
		if err := affected(res, err); err != nil {
			return err
		}
	}
	return tx.Commit()
}
