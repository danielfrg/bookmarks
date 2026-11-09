package store

import (
	"context"
	"database/sql"
	"sort"

	"github.com/danielfrg/bookmarks/internal/bookmarks"
)

type execer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// setBookmarkTags replaces the tag links of a bookmark, creating tags as needed.
func setBookmarkTags(ctx context.Context, tx execer, bookmarkID int64, tags []string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM bookmark_tags WHERE bookmark_id=?`, bookmarkID); err != nil {
		return err
	}
	for _, name := range tags {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO tags(name) VALUES(?)`, name); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO bookmark_tags(bookmark_id,tag_id) SELECT ?,id FROM tags WHERE name=?`, bookmarkID, name); err != nil {
			return err
		}
	}
	return nil
}

func bookmarkTags(ctx context.Context, q execer, bookmarkID int64) ([]string, error) {
	rows, err := q.QueryContext(ctx, `SELECT t.name FROM tags t JOIN bookmark_tags bt ON bt.tag_id=t.id WHERE bt.bookmark_id=?`, bookmarkID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tags := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		tags = append(tags, name)
	}
	sort.Strings(tags)
	return tags, rows.Err()
}

func (s *SQLite) ListTags(ctx context.Context) ([]bookmarks.Tag, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT t.id,t.name,(SELECT COUNT(*) FROM bookmark_tags bt WHERE bt.tag_id=t.id) FROM tags t ORDER BY t.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tags := []bookmarks.Tag{}
	for rows.Next() {
		var t bookmarks.Tag
		if err := rows.Scan(&t.ID, &t.Name, &t.Count); err != nil {
			return nil, err
		}
		tags = append(tags, t)
	}
	return tags, rows.Err()
}

// RenameTag renames a tag. When the new name already exists the links are
// merged into that tag and the old one is deleted.
func (s *SQLite) RenameTag(ctx context.Context, id int64, name string) error {
	normalized := bookmarks.NormalizeTags([]string{name})
	if len(normalized) == 0 {
		return ErrNotFound
	}
	name = normalized[0]
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var current string
	if err := tx.QueryRowContext(ctx, `SELECT name FROM tags WHERE id=?`, id).Scan(&current); err != nil {
		if err == sql.ErrNoRows {
			return ErrNotFound
		}
		return err
	}
	if current != name {
		var targetID int64
		err := tx.QueryRowContext(ctx, `SELECT id FROM tags WHERE name=?`, name).Scan(&targetID)
		switch {
		case err == sql.ErrNoRows:
			_, err = tx.ExecContext(ctx, `UPDATE tags SET name=? WHERE id=?`, name, id)
		case err != nil:
		default:
			if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO bookmark_tags(bookmark_id,tag_id) SELECT bookmark_id,? FROM bookmark_tags WHERE tag_id=?`, targetID, id); err == nil {
				_, err = tx.ExecContext(ctx, `DELETE FROM tags WHERE id=?`, id)
			}
		}
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *SQLite) DeleteTag(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM tags WHERE id=?`, id)
	return affected(res, err)
}

func affected(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
