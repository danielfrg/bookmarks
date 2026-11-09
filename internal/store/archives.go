package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/danielfrg/bookmarks/internal/bookmarks"
)

const archiveColumns = `id,bookmark_id,source,content_type,file_name,display_name,file_size,created_at`

func scanArchive(row scanner) (bookmarks.Archive, error) {
	var a bookmarks.Archive
	var source, created string
	if err := row.Scan(&a.ID, &a.BookmarkID, &source, &a.ContentType, &a.FileName, &a.DisplayName, &a.FileSize, &created); err != nil {
		return a, err
	}
	a.Source = bookmarks.ArchiveSource(source)
	var err error
	a.CreatedAt, err = parseTime(created)
	return a, err
}

// AddArchive inserts a and fills ID (and CreatedAt when zero).
func (s *SQLite) AddArchive(ctx context.Context, a *bookmarks.Archive) error {
	if a.CreatedAt.IsZero() {
		a.CreatedAt = now()
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO archives(bookmark_id,source,content_type,file_name,display_name,file_size,created_at) VALUES(?,?,?,?,?,?,?)`,
		a.BookmarkID, string(a.Source), a.ContentType, a.FileName, a.DisplayName, a.FileSize, formatTime(a.CreatedAt))
	if err != nil {
		return err
	}
	a.ID, err = res.LastInsertId()
	return err
}

// ListArchives returns the archives of a bookmark, newest first.
func (s *SQLite) ListArchives(ctx context.Context, bookmarkID int64) ([]bookmarks.Archive, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+archiveColumns+` FROM archives WHERE bookmark_id=? ORDER BY created_at DESC, id DESC`, bookmarkID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []bookmarks.Archive{}
	for rows.Next() {
		a, err := scanArchive(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *SQLite) GetArchive(ctx context.Context, id int64) (bookmarks.Archive, error) {
	a, err := scanArchive(s.db.QueryRowContext(ctx, `SELECT `+archiveColumns+` FROM archives WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return a, ErrNotFound
	}
	return a, err
}

// DeleteArchive removes the row. Reader text is kept; its archive_id becomes NULL.
func (s *SQLite) DeleteArchive(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM archives WHERE id=?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// LatestReadableArchive returns the newest HTML or PDF archive.
func (s *SQLite) LatestReadableArchive(ctx context.Context, bookmarkID int64) (bookmarks.Archive, error) {
	a, err := scanArchive(s.db.QueryRowContext(ctx, `SELECT `+archiveColumns+` FROM archives
 WHERE bookmark_id=? AND content_type IN ('text/html','application/pdf') ORDER BY created_at DESC, id DESC LIMIT 1`, bookmarkID))
	if errors.Is(err, sql.ErrNoRows) {
		return a, ErrNotFound
	}
	return a, err
}

// PutReaderText upserts the reader text and reindexes the bookmark in the same
// transaction. ArchiveID 0 is stored as NULL.
func (s *SQLite) PutReaderText(ctx context.Context, r bookmarks.ReaderText) error {
	if r.ExtractedAt.IsZero() {
		r.ExtractedAt = now()
	}
	var archiveID any
	if r.ArchiveID != 0 {
		archiveID = r.ArchiveID
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO reader_texts(bookmark_id,archive_id,status,title,markdown,error,word_count,extracted_at) VALUES(?,?,?,?,?,?,?,?)
 ON CONFLICT(bookmark_id) DO UPDATE SET archive_id=excluded.archive_id,status=excluded.status,title=excluded.title,markdown=excluded.markdown,error=excluded.error,word_count=excluded.word_count,extracted_at=excluded.extracted_at`,
		r.BookmarkID, archiveID, string(r.Status), r.Title, r.Markdown, r.Error, r.WordCount, formatTime(r.ExtractedAt)); err != nil {
		return err
	}
	if err := s.reindex(ctx, tx, r.BookmarkID); err != nil {
		return err
	}
	return tx.Commit()
}

// GetReaderText returns the reader text; Status is "none" with a nil error
// when none is stored.
func (s *SQLite) GetReaderText(ctx context.Context, bookmarkID int64) (bookmarks.ReaderText, error) {
	r := bookmarks.ReaderText{BookmarkID: bookmarkID, Status: bookmarks.ReaderNone}
	var archiveID sql.NullInt64
	var status, extracted string
	err := s.db.QueryRowContext(ctx, `SELECT archive_id,status,title,markdown,error,word_count,extracted_at FROM reader_texts WHERE bookmark_id=?`, bookmarkID).
		Scan(&archiveID, &status, &r.Title, &r.Markdown, &r.Error, &r.WordCount, &extracted)
	if errors.Is(err, sql.ErrNoRows) {
		return r, nil
	}
	if err != nil {
		return r, err
	}
	r.ArchiveID, r.Status = archiveID.Int64, bookmarks.ReaderStatus(status)
	r.ExtractedAt, err = parseTime(extracted)
	return r, err
}
