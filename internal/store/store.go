// Package store persists bookmarks in SQLite. All SQL lives here.
package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

var (
	ErrNotFound = errors.New("Not found")
	ErrConflict = errors.New("A bookmark with that URL already exists")
)

//go:embed migrations/sqlite/*.sql
var migrations embed.FS

// SQLite is the SQLite-backed store.
type SQLite struct{ db *sql.DB }

// Open opens (creating if needed) the database at path and applies migrations.
func Open(path string) (*SQLite, error) {
	if path == "" {
		return nil, errors.New("Database path cannot be empty")
	}
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return nil, fmt.Errorf("create database directory: %w", err)
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	fail := func(err error) (*SQLite, error) { db.Close(); return nil, fmt.Errorf("open database: %w", err) }
	if err := db.Ping(); err != nil {
		return fail(err)
	}
	for _, statement := range []string{"PRAGMA busy_timeout=5000", "PRAGMA journal_mode=DELETE", "PRAGMA synchronous=FULL", "PRAGMA foreign_keys=ON"} {
		if _, err := db.Exec(statement); err != nil {
			return fail(err)
		}
	}
	sub, err := fs.Sub(migrations, "migrations/sqlite")
	if err != nil {
		return fail(err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, db, sub)
	if err != nil {
		return fail(err)
	}
	if _, err := provider.Up(context.Background()); err != nil {
		return fail(fmt.Errorf("migrate: %w", err))
	}
	return &SQLite{db: db}, nil
}

func (s *SQLite) Close() error                   { return s.db.Close() }
func (s *SQLite) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

type scanner interface{ Scan(...any) error }

// Fixed-width RFC 3339 UTC so text ordering matches time ordering.
const timeLayout = "2006-01-02T15:04:05.000000000Z"

func formatTime(t time.Time) string { return t.UTC().Format(timeLayout) }

func parseTime(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, s)
	return t.UTC(), err
}

func now() time.Time { return time.Now().UTC() }

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func isUnique(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

func joinTags(tags []string) string { return strings.Join(tags, " ") }
func splitTags(s string) []string   { return strings.Fields(s) }
