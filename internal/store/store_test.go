package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
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
