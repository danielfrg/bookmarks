package main

import (
	"path/filepath"
	"testing"
)

func clearEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"BOOKMARKS_ADDR", "BOOKMARKS_DATA_DIR", "BOOKMARKS_PUBLIC_URL",
		"BOOKMARKS_MCP_ALLOWED_HOSTS", "BOOKMARKS_MAX_UPLOAD_MB",
		"BOOKMARKS_FAVICON_PROVIDER", "BOOKMARKS_ALLOW_PRIVATE_FETCH",
	} {
		t.Setenv(name, "")
	}
}

func TestLoadSettingsDefaults(t *testing.T) {
	clearEnv(t)
	s, err := loadSettings()
	if err != nil {
		t.Fatal(err)
	}
	if s.Addr != "127.0.0.1:8080" || s.DatabasePath != filepath.Join("data", "bookmarks.db") || s.MaxUploadBytes != 100<<20 {
		t.Fatalf("%+v", s)
	}
	if s.PublicURL != "http://127.0.0.1:8080" || s.AllowPrivateFetch || s.FaviconProvider == "" {
		t.Fatalf("%+v", s)
	}
}

func TestLoadSettingsRejectsBadPublicURL(t *testing.T) {
	clearEnv(t)
	t.Setenv("BOOKMARKS_PUBLIC_URL", "http://x/path")
	if _, err := loadSettings(); err == nil {
		t.Fatal("accepted public URL with path")
	}
}

func TestLoadSettingsOtherValues(t *testing.T) {
	clearEnv(t)
	t.Setenv("BOOKMARKS_DATA_DIR", "/srv/bm")
	t.Setenv("BOOKMARKS_MCP_ALLOWED_HOSTS", "a.example, b.example")
	t.Setenv("BOOKMARKS_MAX_UPLOAD_MB", "5")
	t.Setenv("BOOKMARKS_ALLOW_PRIVATE_FETCH", "true")
	s, err := loadSettings()
	if err != nil {
		t.Fatal(err)
	}
	if s.DatabasePath != "/srv/bm/bookmarks.db" || s.MaxUploadBytes != 5<<20 || !s.AllowPrivateFetch ||
		len(s.MCPAllowedHosts) != 2 || s.MCPAllowedHosts[1] != "b.example" {
		t.Fatalf("%+v", s)
	}
	for name, value := range map[string]string{
		"BOOKMARKS_MAX_UPLOAD_MB": "0", "BOOKMARKS_ALLOW_PRIVATE_FETCH": "maybe", "BOOKMARKS_ADDR": "nohost", "BOOKMARKS_MCP_ALLOWED_HOSTS": "https://a.example",
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv(name, value)
			if _, err := loadSettings(); err == nil {
				t.Fatal("accepted", value)
			}
		})
	}
}
