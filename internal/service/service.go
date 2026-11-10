// Package service holds the business rules of the bookmark manager. The REST
// API and the MCP server call only this package.
package service

import (
	"container/list"
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/danielfrg/bookmarks/internal/archive"
	"github.com/danielfrg/bookmarks/internal/bookmarks"
	"github.com/danielfrg/bookmarks/internal/capture"
	"github.com/danielfrg/bookmarks/internal/fetch"
	"github.com/danielfrg/bookmarks/internal/jobs"
	"github.com/danielfrg/bookmarks/internal/store"
)

// Store is the persistence the service needs; *store.SQLite implements it.
type Store interface {
	GetBookmark(ctx context.Context, id int64) (bookmarks.Bookmark, error)
	GetBookmarkByURL(ctx context.Context, rawURL string) (bookmarks.Bookmark, error)
	CreateBookmark(ctx context.Context, b *bookmarks.Bookmark) error
	UpdateBookmark(ctx context.Context, b bookmarks.Bookmark) error
	DeleteBookmark(ctx context.Context, id int64) error
	CountBookmarksWithPreview(ctx context.Context, name string) (int, error)
	Search(ctx context.Context, q store.SearchQuery) (store.SearchResult, error)

	AddArchive(ctx context.Context, a *bookmarks.Archive) error
	ListArchives(ctx context.Context, bookmarkID int64) ([]bookmarks.Archive, error)
	GetArchive(ctx context.Context, id int64) (bookmarks.Archive, error)
	DeleteArchive(ctx context.Context, id int64) error
	LatestReadableArchive(ctx context.Context, bookmarkID int64) (bookmarks.Archive, error)
	PutReaderText(ctx context.Context, r bookmarks.ReaderText) error
	GetReaderText(ctx context.Context, bookmarkID int64) (bookmarks.ReaderText, error)

	ListTags(ctx context.Context) ([]bookmarks.Tag, error)
	RenameTag(ctx context.Context, id int64, name string) error
	DeleteTag(ctx context.Context, id int64) error

	ListSavedSearches(ctx context.Context) ([]bookmarks.SavedSearch, error)
	GetSavedSearch(ctx context.Context, id int64) (bookmarks.SavedSearch, error)
	SaveSavedSearch(ctx context.Context, b *bookmarks.SavedSearch) error
	DeleteSavedSearch(ctx context.Context, id int64) error
	ReorderSavedSearches(ctx context.Context, ids []int64) error

	GetSettings(ctx context.Context) (bookmarks.Settings, error)
	PutSettings(ctx context.Context, v bookmarks.Settings) error

	CreateAPIToken(ctx context.Context, t *bookmarks.APIToken) error
	ListAPITokens(ctx context.Context) ([]bookmarks.APIToken, error)
	DeleteAPIToken(ctx context.Context, id int64) error
	FindAPITokenByHash(ctx context.Context, hash string) (bookmarks.APIToken, error)
	TouchAPIToken(ctx context.Context, id int64, at time.Time) error
}

var _ Store = (*store.SQLite)(nil)

// Enqueuer schedules background jobs; *jobs.Queue implements it.
type Enqueuer interface {
	Enqueue(ctx context.Context, kind jobs.Kind, bookmarkID int64) error
}

var _ Enqueuer = (*jobs.Queue)(nil)

// Options configures a Service.
type Options struct {
	DataDir         string
	FaviconProvider string // template containing "{url}"; empty disables favicons
	MaxUploadBytes  int64  // uncompressed archive size limit
	Now             func() time.Time
	Logger          *log.Logger // job warnings; defaults to the standard logger
}

// ValidationError reports invalid input. Field names the offending input.
type ValidationError struct{ Field, Message string }

func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }

func invalid(field, msg string) error { return &ValidationError{Field: field, Message: msg} }

// Service implements the business rules on top of the store.
type Service struct {
	st    Store
	fc    *fetch.Client
	cap   capture.Capturer
	q     Enqueuer
	o     Options
	log   *log.Logger
	files archive.Files

	waybackBase string // https://web.archive.org; overridden in tests
	meta        *metadataCache
}

const defaultMaxUploadBytes = 100 << 20

// New builds a Service. q may be nil and set later with SetEnqueuer.
func New(st Store, fc *fetch.Client, cap capture.Capturer, q Enqueuer, o Options) *Service {
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.MaxUploadBytes <= 0 {
		o.MaxUploadBytes = defaultMaxUploadBytes
	}
	lg := o.Logger
	if lg == nil {
		lg = log.Default()
	}
	return &Service{
		st: st, fc: fc, cap: cap, q: q, o: o, log: lg,
		files:       archive.Files{Dir: filepath.Join(o.DataDir, "archives")},
		waybackBase: "https://web.archive.org",
		meta:        newMetadataCache(10),
	}
}

// SetEnqueuer sets the job queue. Call it before serving requests: the queue
// is built after the service because it needs JobHandlers.
func (s *Service) SetEnqueuer(q Enqueuer) { s.q = q }

func (s *Service) now() time.Time { return s.o.Now().UTC() }

func (s *Service) faviconDir() string { return filepath.Join(s.o.DataDir, "favicons") }
func (s *Service) previewDir() string { return filepath.Join(s.o.DataDir, "previews") }

// enqueue schedules a job, logging failures: the triggering write already
// succeeded and the job can be requested again.
func (s *Service) enqueue(ctx context.Context, kind jobs.Kind, id int64) {
	if err := s.enqueueErr(ctx, kind, id); err != nil {
		s.log.Printf("service: enqueue %s for bookmark %d: %v", kind, id, err)
	}
}

func (s *Service) enqueueErr(ctx context.Context, kind jobs.Kind, id int64) error {
	if s.q == nil {
		return errNoQueue
	}
	return s.q.Enqueue(ctx, kind, id)
}

var errNoQueue = errors.New("job queue not configured")

// Settings returns the stored settings.
func (s *Service) Settings(ctx context.Context) (bookmarks.Settings, error) {
	return s.st.GetSettings(ctx)
}

// PutSettings stores v and returns the stored settings.
func (s *Service) PutSettings(ctx context.Context, v bookmarks.Settings) (bookmarks.Settings, error) {
	if err := s.st.PutSettings(ctx, v); err != nil {
		return bookmarks.Settings{}, err
	}
	return s.st.GetSettings(ctx)
}

// removePreviewIfUnused deletes preview file name when no bookmark uses it.
func (s *Service) removePreviewIfUnused(ctx context.Context, name string) {
	if name == "" || name != filepath.Base(name) {
		return
	}
	n, err := s.st.CountBookmarksWithPreview(ctx, name)
	if err != nil {
		s.log.Printf("service: count users of preview %s: %v", name, err)
		return
	}
	if n == 0 {
		s.removeFile(filepath.Join(s.previewDir(), name))
	}
}

// removeFile deletes path, ignoring a missing file and logging other errors.
func (s *Service) removeFile(path string) {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		s.log.Printf("service: remove %s: %v", path, err)
	}
}

// metadataCache is a small LRU of page metadata keyed by URL.
type metadataCache struct {
	mu    sync.Mutex
	size  int
	order *list.List // front = most recent; values are *metaEntry
	items map[string]*list.Element
}

type metaEntry struct {
	url string
	md  fetch.Metadata
}

func newMetadataCache(size int) *metadataCache {
	return &metadataCache{size: size, order: list.New(), items: map[string]*list.Element{}}
}

func (c *metadataCache) get(url string) (fetch.Metadata, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[url]
	if !ok {
		return fetch.Metadata{}, false
	}
	c.order.MoveToFront(el)
	return el.Value.(*metaEntry).md, true
}

func (c *metadataCache) put(url string, md fetch.Metadata) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[url]; ok {
		el.Value.(*metaEntry).md = md
		c.order.MoveToFront(el)
		return
	}
	c.items[url] = c.order.PushFront(&metaEntry{url, md})
	if c.order.Len() > c.size {
		last := c.order.Back()
		c.order.Remove(last)
		delete(c.items, last.Value.(*metaEntry).url)
	}
}
