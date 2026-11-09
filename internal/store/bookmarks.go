package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/danielfrg/bookmarks/internal/bookmarks"
)

const bookmarkColumns = `b.id,b.url,b.title,b.description,b.notes,b.unread,b.is_archived,
 b.favicon_file,b.preview_file,b.web_archive_url,b.date_added,b.date_modified,
 (SELECT COUNT(*) FROM archives a WHERE a.bookmark_id=b.id),
 (SELECT COUNT(*) FROM archives a WHERE a.bookmark_id=b.id AND a.source<>'server'),
 COALESCE((SELECT r.status FROM reader_texts r WHERE r.bookmark_id=b.id),'none')`

const selectBookmark = `SELECT ` + bookmarkColumns + ` FROM bookmarks b`

func scanBookmark(row scanner) (bookmarks.Bookmark, error) {
	var b bookmarks.Bookmark
	var unread, archived, nonServer int
	var added, modified, reader string
	err := row.Scan(&b.ID, &b.URL, &b.Title, &b.Description, &b.Notes, &unread, &archived,
		&b.FaviconFile, &b.PreviewFile, &b.WebArchiveURL, &added, &modified,
		&b.ArchiveCount, &nonServer, &reader)
	if errors.Is(err, sql.ErrNoRows) {
		return b, ErrNotFound
	}
	if err != nil {
		return b, err
	}
	b.Unread, b.Archived = unread != 0, archived != 0
	b.BasicCaptureOnly = b.ArchiveCount > 0 && nonServer == 0
	b.ReaderStatus = bookmarks.ReaderStatus(reader)
	if b.DateAdded, err = parseTime(added); err != nil {
		return b, err
	}
	b.DateModified, err = parseTime(modified)
	return b, err
}

func (s *SQLite) loadBookmark(ctx context.Context, where string, arg any) (bookmarks.Bookmark, error) {
	b, err := scanBookmark(s.db.QueryRowContext(ctx, selectBookmark+" WHERE "+where, arg))
	if err != nil {
		return b, err
	}
	b.Tags, err = bookmarkTags(ctx, s.db, b.ID)
	return b, err
}

func (s *SQLite) GetBookmark(ctx context.Context, id int64) (bookmarks.Bookmark, error) {
	return s.loadBookmark(ctx, "b.id=?", id)
}

func (s *SQLite) GetBookmarkByURL(ctx context.Context, rawURL string) (bookmarks.Bookmark, error) {
	return s.loadBookmark(ctx, "b.url_normalized=?", bookmarks.NormalizeURL(rawURL))
}

// CreateBookmark inserts b and sets b.ID. Zero dates default to now.
func (s *SQLite) CreateBookmark(ctx context.Context, b *bookmarks.Bookmark) error {
	if b.DateAdded.IsZero() {
		b.DateAdded = now()
	}
	if b.DateModified.IsZero() {
		b.DateModified = b.DateAdded
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `INSERT INTO bookmarks(url,url_normalized,title,description,notes,unread,is_archived,favicon_file,preview_file,web_archive_url,date_added,date_modified)
 VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, b.URL, bookmarks.NormalizeURL(b.URL), b.Title, b.Description, b.Notes,
		boolInt(b.Unread), boolInt(b.Archived), b.FaviconFile, b.PreviewFile, b.WebArchiveURL,
		formatTime(b.DateAdded), formatTime(b.DateModified))
	if isUnique(err) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	if err := setBookmarkTags(ctx, tx, id, b.Tags); err != nil {
		return err
	}
	if err := s.reindex(ctx, tx, id); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	b.ID = id
	return nil
}

// UpdateBookmark replaces every stored field of the bookmark, including tags.
func (s *SQLite) UpdateBookmark(ctx context.Context, b bookmarks.Bookmark) error {
	if b.DateModified.IsZero() {
		b.DateModified = now()
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE bookmarks SET url=?,url_normalized=?,title=?,description=?,notes=?,unread=?,is_archived=?,
 favicon_file=?,preview_file=?,web_archive_url=?,date_added=?,date_modified=? WHERE id=?`,
		b.URL, bookmarks.NormalizeURL(b.URL), b.Title, b.Description, b.Notes, boolInt(b.Unread), boolInt(b.Archived),
		b.FaviconFile, b.PreviewFile, b.WebArchiveURL, formatTime(b.DateAdded), formatTime(b.DateModified), b.ID)
	if isUnique(err) {
		return ErrConflict
	}
	if err := affected(res, err); err != nil {
		return err
	}
	if err := setBookmarkTags(ctx, tx, b.ID, b.Tags); err != nil {
		return err
	}
	if err := s.reindex(ctx, tx, b.ID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLite) DeleteBookmark(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `DELETE FROM bookmarks WHERE id=?`, id)
	if err := affected(res, err); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM bookmarks_fts WHERE rowid=?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// CountBookmarksWithPreview returns how many bookmarks use the preview file
// name (preview files are shared by bookmarks with the same image URL).
func (s *SQLite) CountBookmarksWithPreview(ctx context.Context, name string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM bookmarks WHERE preview_file=?`, name).Scan(&n)
	return n, err
}
