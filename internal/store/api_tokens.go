package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/danielfrg/bookmarks/internal/bookmarks"
)

const selectAPIToken = `SELECT id,name,token_hash,prefix,created_at,last_used_at FROM api_tokens`

func scanAPIToken(row scanner) (bookmarks.APIToken, error) {
	var t bookmarks.APIToken
	var created string
	var used sql.NullString
	err := row.Scan(&t.ID, &t.Name, &t.TokenHash, &t.Prefix, &created, &used)
	if errors.Is(err, sql.ErrNoRows) {
		return t, ErrNotFound
	}
	if err != nil {
		return t, err
	}
	if t.CreatedAt, err = parseTime(created); err != nil {
		return t, err
	}
	if used.Valid {
		u, err := parseTime(used.String)
		if err != nil {
			return t, err
		}
		t.LastUsedAt = &u
	}
	return t, nil
}

// CreateAPIToken inserts t and sets t.ID. A duplicate TokenHash is an error.
func (s *SQLite) CreateAPIToken(ctx context.Context, t *bookmarks.APIToken) error {
	res, err := s.db.ExecContext(ctx, `INSERT INTO api_tokens(name,token_hash,prefix,created_at) VALUES(?,?,?,?)`,
		t.Name, t.TokenHash, t.Prefix, formatTime(t.CreatedAt))
	if err != nil {
		return err
	}
	t.ID, err = res.LastInsertId()
	return err
}

// ListAPITokens returns every token, newest first.
func (s *SQLite) ListAPITokens(ctx context.Context) ([]bookmarks.APIToken, error) {
	rows, err := s.db.QueryContext(ctx, selectAPIToken+" ORDER BY created_at DESC, id DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []bookmarks.APIToken{}
	for rows.Next() {
		t, err := scanAPIToken(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, t)
	}
	return list, rows.Err()
}

func (s *SQLite) DeleteAPIToken(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM api_tokens WHERE id=?`, id)
	return affected(res, err)
}

// FindAPITokenByHash returns the token whose TokenHash is hash, or ErrNotFound.
func (s *SQLite) FindAPITokenByHash(ctx context.Context, hash string) (bookmarks.APIToken, error) {
	return scanAPIToken(s.db.QueryRowContext(ctx, selectAPIToken+" WHERE token_hash=?", hash))
}

// TouchAPIToken sets the last-used time of token id.
func (s *SQLite) TouchAPIToken(ctx context.Context, id int64, at time.Time) error {
	res, err := s.db.ExecContext(ctx, `UPDATE api_tokens SET last_used_at=? WHERE id=?`, formatTime(at), id)
	return affected(res, err)
}
