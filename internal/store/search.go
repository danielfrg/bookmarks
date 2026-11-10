package store

import (
	"context"
	"database/sql"
	"strings"
	"unicode"

	"github.com/danielfrg/bookmarks/internal/bookmarks"
	"github.com/danielfrg/bookmarks/internal/query"
)

// SearchQuery describes a bookmark search.
type SearchQuery struct {
	Expr          query.Expr
	Scope         string // "active" (default), "archived", "all"
	Unread        string // "", "yes", "no"
	Sort          string // added_desc (default), added_asc, modified_desc, modified_asc, title_asc, title_desc, relevance
	Limit, Offset int    // Limit 0 -> 50, max 500
	Snippets      bool
}

// SearchHit is a bookmark plus an optional highlighted snippet.
type SearchHit struct {
	bookmarks.Bookmark
	Snippet string
}

// SearchResult holds one page of hits and the total match count.
type SearchResult struct {
	Total int
	Hits  []SearchHit
}

const (
	defaultLimit = 50
	maxLimit     = 500
)

// reindex rewrites the FTS row of a bookmark from its current fields, tag
// names and reader text. It must run in the same transaction as the write.
func (s *SQLite) reindex(ctx context.Context, tx *sql.Tx, bookmarkID int64) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM bookmarks_fts WHERE rowid=?`, bookmarkID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO bookmarks_fts(rowid,title,description,notes,url,tags,reader)
 SELECT b.id,b.title,b.description,b.notes,b.url,
  COALESCE((SELECT group_concat(t.name,' ') FROM tags t JOIN bookmark_tags bt ON bt.tag_id=t.id WHERE bt.bookmark_id=b.id),''),
  COALESCE((SELECT r.markdown FROM reader_texts r WHERE r.bookmark_id=b.id),'')
 FROM bookmarks b WHERE b.id=?`, bookmarkID)
	return err
}

// ftsTerm builds a safe FTS5 MATCH expression: every token is double-quoted,
// so no user input is interpreted as FTS syntax. Returns "" when text has no
// tokens.
func ftsTerm(text string, phrase bool) string {
	tokens := strings.FieldsFunc(text, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	if len(tokens) == 0 {
		return ""
	}
	quote := func(t string) string { return `"` + strings.ReplaceAll(t, `"`, `""`) + `"` }
	if phrase {
		return quote(strings.Join(tokens, " "))
	}
	quoted := make([]string, len(tokens))
	for i, t := range tokens {
		quoted[i] = quote(t)
	}
	quoted[len(quoted)-1] += "*"
	return strings.Join(quoted, " ")
}

// compiler turns an Expr into a SQL condition plus bound arguments.
type compiler struct{ args []any }

func (c *compiler) compile(e query.Expr) string {
	switch v := e.(type) {
	case nil:
		return "1=1"
	case query.Term:
		m := ftsTerm(v.Text, v.Phrase)
		if m == "" {
			return "1=1"
		}
		c.args = append(c.args, m)
		return `b.id IN (SELECT rowid FROM bookmarks_fts WHERE bookmarks_fts MATCH ?)`
	case query.Tag:
		c.args = append(c.args, v.Name)
		return `EXISTS (SELECT 1 FROM bookmark_tags bt JOIN tags t ON t.id=bt.tag_id WHERE bt.bookmark_id=b.id AND t.name=?)`
	case query.Keyword:
		switch v.Name {
		case "unread":
			return `b.unread=1`
		case "untagged":
			return `NOT EXISTS (SELECT 1 FROM bookmark_tags bt WHERE bt.bookmark_id=b.id)`
		case "basic":
			return `(EXISTS (SELECT 1 FROM archives a WHERE a.bookmark_id=b.id) AND NOT EXISTS (SELECT 1 FROM archives a WHERE a.bookmark_id=b.id AND a.source<>'server'))`
		case "noarchive":
			return `NOT EXISTS (SELECT 1 FROM archives a WHERE a.bookmark_id=b.id)`
		}
		return "1=1"
	case query.Not:
		return "NOT (" + c.compile(v.X) + ")"
	case query.And:
		l := c.compile(v.L)
		return "(" + l + " AND " + c.compile(v.R) + ")"
	case query.Or:
		l := c.compile(v.L)
		return "(" + l + " OR " + c.compile(v.R) + ")"
	}
	return "1=1"
}

// rankExpr collects the MATCH expressions of terms that every result must
// match (top-level AND chain only), so joining on it never changes the result
// set, only enables bm25 and snippet.
func rankExpr(e query.Expr) string {
	var parts []string
	var walk func(query.Expr)
	walk = func(e query.Expr) {
		switch v := e.(type) {
		case query.Term:
			if m := ftsTerm(v.Text, v.Phrase); m != "" {
				parts = append(parts, m)
			}
		case query.And:
			walk(v.L)
			walk(v.R)
		}
	}
	walk(e)
	return strings.Join(parts, " ")
}

// Search runs q. Tags are loaded for the page with one extra query.
func (s *SQLite) Search(ctx context.Context, q SearchQuery) (SearchResult, error) {
	limit := q.Limit
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	offset := max(q.Offset, 0)

	var c compiler
	where := []string{c.compile(q.Expr)}
	switch q.Scope {
	case "archived":
		where = append(where, "b.is_archived=1")
	case "all":
	default:
		where = append(where, "b.is_archived=0")
	}
	switch q.Unread {
	case "yes":
		where = append(where, "b.unread=1")
	case "no":
		where = append(where, "b.unread=0")
	}
	whereSQL := strings.Join(where, " AND ")

	var res SearchResult
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM bookmarks b WHERE `+whereSQL, c.args...).Scan(&res.Total); err != nil {
		return res, err
	}

	rank := rankExpr(q.Expr)
	cols, from, args := bookmarkColumns, " FROM bookmarks b", append([]any(nil), c.args...)
	snippets := q.Snippets && rank != ""
	if rank != "" {
		from += " JOIN bookmarks_fts ON bookmarks_fts.rowid=b.id"
		whereSQL += " AND bookmarks_fts MATCH ?"
		args = append(args, rank)
		if snippets {
			cols += `, snippet(bookmarks_fts, -1, '**', '**', '…', 24)`
		}
	}
	var order string
	switch q.Sort {
	case "added_asc":
		order = "b.date_added ASC, b.id ASC"
	case "modified_desc":
		order = "b.date_modified DESC, b.id DESC"
	case "modified_asc":
		order = "b.date_modified ASC, b.id ASC"
	case "title_asc":
		order = "b.title COLLATE NOCASE ASC, b.id ASC"
	case "title_desc":
		order = "b.title COLLATE NOCASE DESC, b.id DESC"
	case "relevance":
		if rank != "" {
			order = "bm25(bookmarks_fts, 10, 5, 5, 2, 8, 1), b.date_added DESC, b.id DESC"
		} else {
			order = "b.date_added DESC, b.id DESC"
		}
	default:
		order = "b.date_added DESC, b.id DESC"
	}
	args = append(args, limit, offset)
	rows, err := s.db.QueryContext(ctx, "SELECT "+cols+from+" WHERE "+whereSQL+" ORDER BY "+order+" LIMIT ? OFFSET ?", args...)
	if err != nil {
		return res, err
	}
	// Collect before running other queries: there is a single connection.
	res.Hits = []SearchHit{}
	for rows.Next() {
		var h SearchHit
		var sc scanner = rows
		if snippets {
			sc = snippetScanner{rows, &h.Snippet}
		}
		if h.Bookmark, err = scanBookmark(sc); err != nil {
			rows.Close()
			return res, err
		}
		res.Hits = append(res.Hits, h)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return res, err
	}
	rows.Close()
	return res, s.attachTags(ctx, res.Hits)
}

// snippetScanner scans the bookmark columns followed by one snippet column.
type snippetScanner struct {
	rows    *sql.Rows
	snippet *string
}

func (s snippetScanner) Scan(dest ...any) error { return s.rows.Scan(append(dest, s.snippet)...) }

// attachTags loads the tags of all hits with a single query.
func (s *SQLite) attachTags(ctx context.Context, hits []SearchHit) error {
	if len(hits) == 0 {
		return nil
	}
	idx := make(map[int64]int, len(hits))
	args := make([]any, len(hits))
	for i := range hits {
		hits[i].Tags = []string{}
		idx[hits[i].ID] = i
		args[i] = hits[i].ID
	}
	rows, err := s.db.QueryContext(ctx, `SELECT bt.bookmark_id,t.name FROM bookmark_tags bt JOIN tags t ON t.id=bt.tag_id
 WHERE bt.bookmark_id IN (`+strings.TrimSuffix(strings.Repeat("?,", len(args)), ",")+`) ORDER BY t.name`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return err
		}
		hits[idx[id]].Tags = append(hits[idx[id]].Tags, name)
	}
	return rows.Err()
}
