// Package bookmarks holds the pure domain types and normalization helpers
// shared by the store, search, service and API layers.
package bookmarks

import (
	"encoding/json"
	"time"
)

type ReaderStatus string

const (
	ReaderOK     ReaderStatus = "ok"
	ReaderFailed ReaderStatus = "failed"
	ReaderNone   ReaderStatus = "none"
)

type ArchiveSource string

const (
	SourceExtension ArchiveSource = "extension"
	SourceServer    ArchiveSource = "server"
	SourceUpload    ArchiveSource = "upload"
	SourceImport    ArchiveSource = "import" // from a one-time migration; not accepted for new archives
)

type Bookmark struct {
	ID          int64
	URL         string
	Title       string
	Description string
	Notes       string
	Tags        []string // sorted, lower-case
	Unread      bool

	FaviconFile   string
	PreviewFile   string
	WebArchiveURL string

	DateAdded    time.Time
	DateModified time.Time

	// Read-only, filled by store queries.
	ArchiveCount     int
	BasicCaptureOnly bool  // at least one archive and every archive has source "server"
	LatestArchiveID  int64 // newest archive (as listed in the details), 0 when none
	ReaderStatus     ReaderStatus
}

type Archive struct {
	ID          int64
	BookmarkID  int64
	Source      ArchiveSource
	ContentType string
	FileName    string
	DisplayName string
	FileSize    int64
	CreatedAt   time.Time
}

type ReaderText struct {
	BookmarkID  int64
	ArchiveID   int64
	Status      ReaderStatus
	Title       string
	Markdown    string
	Error       string
	WordCount   int
	ExtractedAt time.Time
}

type Tag struct {
	ID    int64
	Name  string
	Count int
}

type SavedSearch struct {
	ID           int64
	Name         string
	Query        string
	AnyTags      []string
	AllTags      []string
	ExcludedTags []string
	FilterUnread string // "off", "yes" or "no"
	Order        int
	DateCreated  time.Time
	DateModified time.Time
}

// APIToken is a stored API token. The plaintext is never stored: TokenHash is
// the hex SHA-256 of the full token and Prefix the start of its random part.
type APIToken struct {
	ID         int64
	Name       string
	TokenHash  string
	Prefix     string
	CreatedAt  time.Time
	LastUsedAt *time.Time
}

type Settings struct {
	FaviconsEnabled bool
	PreviewsEnabled bool
	WaybackEnabled  bool
	DefaultUnread   bool
	AutoTagRules    string
	UI              json.RawMessage
}

// DefaultSettings returns the settings used before anything is saved.
func DefaultSettings() Settings {
	return Settings{
		FaviconsEnabled: true,
		PreviewsEnabled: true,
		UI:              json.RawMessage(`{}`),
	}
}
