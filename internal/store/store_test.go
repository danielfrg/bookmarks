package store

import (
	"context"
	"database/sql"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
)

func openTest(t *testing.T) *SQLite {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "sub", "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestOpenAppliesPragmasAndMigrations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	var mode string
	if err := s.db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil || mode != "delete" {
		t.Fatalf("journal_mode = %q, %v", mode, err)
	}
	var fk int
	if err := s.db.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil || fk != 1 {
		t.Fatalf("foreign_keys = %d, %v", fk, err)
	}
	if err := s.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s2, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	s2.Close()
}

func TestOpenPathNeedingEscapes(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "my data #1 ?x%41")
	s, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := os.Stat(filepath.Join(dir, "test.db")); err != nil {
		t.Fatalf("database not at requested path: %v", err)
	}
	var fk int
	if err := s.db.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil || fk != 1 {
		t.Fatalf("foreign_keys = %d, %v", fk, err)
	}
}

func TestOpenRelativePath(t *testing.T) {
	t.Chdir(t.TempDir())
	s, err := Open("data/rel.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := os.Stat("data/rel.db"); err != nil {
		t.Fatal(err)
	}
}

func TestMigrationDropsIsArchivedFromExistingDB(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	sub, err := fs.Sub(migrations, "migrations/sqlite")
	if err != nil {
		t.Fatal(err)
	}
	p, err := goose.NewProvider(goose.DialectSQLite3, db, sub)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := p.UpTo(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO bookmarks(url,url_normalized,title,is_archived,date_added,date_modified)
 VALUES('https://old.example/','https://old.example/','Old',1,'2026-01-01T00:00:00.000000000Z','2026-01-01T00:00:00.000000000Z')`); err != nil {
		t.Fatal(err)
	}
	db.Close()

	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if hasColumn(t, s.db, "bookmarks", "is_archived") {
		t.Fatal("is_archived still present")
	}
	r, err := s.Search(ctx, SearchQuery{})
	if err != nil || r.Total != 1 || r.Hits[0].Title != "Old" {
		t.Fatalf("search after migration: %+v %v", r, err)
	}

	// Down restores the column.
	p, err = goose.NewProvider(goose.DialectSQLite3, s.db, sub)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Down(ctx); err != nil {
		t.Fatal(err)
	}
	if !hasColumn(t, s.db, "bookmarks", "is_archived") {
		t.Fatal("down did not restore is_archived")
	}
}

func TestFreshDBHasNoIsArchived(t *testing.T) {
	if hasColumn(t, openTest(t).db, "bookmarks", "is_archived") {
		t.Fatal("is_archived present on a fresh database")
	}
}

func hasColumn(t *testing.T, db *sql.DB, table, col string) bool {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name=?`, table, col).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n > 0
}
