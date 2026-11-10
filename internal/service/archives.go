package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/danielfrg/bookmarks/internal/bookmarks"
	"github.com/danielfrg/bookmarks/internal/jobs"
	"github.com/danielfrg/bookmarks/internal/store"
)

// ArchiveInput is a new archive file, created now.
type ArchiveInput struct {
	Source                   bookmarks.ArchiveSource
	ContentType, DisplayName string
	Body                     io.Reader
}

// validSource reports whether a new archive may have source s. "import" only
// labels archives from a one-time migration.
func validSource(s bookmarks.ArchiveSource) bool {
	switch s {
	case bookmarks.SourceExtension, bookmarks.SourceServer, bookmarks.SourceUpload:
		return true
	}
	return false
}

// mediaType reduces a Content-Type to its lower-case media type, so the store
// recognises readable archives ("text/html; charset=utf-8" → "text/html").
func mediaType(ct string) string {
	if mt, _, err := mime.ParseMediaType(ct); err == nil {
		return mt
	}
	if ct = strings.TrimSpace(ct); ct == "" {
		return "application/octet-stream"
	}
	return strings.ToLower(ct)
}

// randomSuffix keeps archive names unique when two share a timestamp.
func randomSuffix() string {
	var b [3]byte
	rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// AddArchive stores in.Body as a gzipped file and records it, then schedules
// reader text extraction. A body larger than MaxUploadBytes returns
// archive.ErrTooLarge.
func (s *Service) AddArchive(ctx context.Context, bookmarkID int64, in ArchiveInput) (bookmarks.Archive, error) {
	if !validSource(in.Source) {
		return bookmarks.Archive{}, invalid("source", `must be "extension", "server" or "upload"`)
	}
	if in.Body == nil {
		return bookmarks.Archive{}, invalid("body", "is required")
	}
	if _, err := s.st.GetBookmark(ctx, bookmarkID); err != nil {
		return bookmarks.Archive{}, err
	}
	now := s.now()
	a := bookmarks.Archive{
		BookmarkID:  bookmarkID,
		Source:      in.Source,
		ContentType: mediaType(in.ContentType),
		DisplayName: in.DisplayName,
		FileName:    fmt.Sprintf("%d/%d-%s-%s.gz", bookmarkID, now.UnixNano(), in.Source, randomSuffix()),
		CreatedAt:   now,
	}
	size, err := s.files.Write(a.FileName, in.Body, s.o.MaxUploadBytes)
	if err != nil {
		return bookmarks.Archive{}, err
	}
	a.FileSize = size
	if err := s.st.AddArchive(ctx, &a); err != nil {
		if rerr := s.files.Remove(a.FileName); rerr != nil {
			s.log.Printf("service: remove orphaned archive file of bookmark %d: %v", bookmarkID, rerr)
		}
		return bookmarks.Archive{}, err
	}
	s.enqueue(ctx, jobs.KindExtract, bookmarkID)
	return a, nil
}

// OpenArchive returns the archive and its decompressed content. A missing
// file is reported as store.ErrNotFound.
func (s *Service) OpenArchive(ctx context.Context, id int64) (bookmarks.Archive, io.ReadCloser, error) {
	a, err := s.st.GetArchive(ctx, id)
	if err != nil {
		return bookmarks.Archive{}, nil, err
	}
	rc, err := s.files.Open(a.FileName)
	if errors.Is(err, os.ErrNotExist) {
		s.log.Printf("service: archive %d file is missing", id)
		return bookmarks.Archive{}, nil, store.ErrNotFound
	}
	if err != nil {
		return bookmarks.Archive{}, nil, err
	}
	return a, rc, nil
}

// DeleteArchive removes the archive row and its file. The reader text is kept.
func (s *Service) DeleteArchive(ctx context.Context, id int64) error {
	a, err := s.st.GetArchive(ctx, id)
	if err != nil {
		return err
	}
	if err := s.st.DeleteArchive(ctx, id); err != nil {
		return err
	}
	if err := s.files.Remove(a.FileName); err != nil {
		s.log.Printf("service: remove archive %d file: %v", id, err)
	}
	return nil
}

// RequestCapture schedules a server capture of bookmark id.
func (s *Service) RequestCapture(ctx context.Context, id int64) error {
	return s.request(ctx, jobs.KindCapture, id)
}

// RequestExtract schedules reader text extraction for bookmark id.
func (s *Service) RequestExtract(ctx context.Context, id int64) error {
	return s.request(ctx, jobs.KindExtract, id)
}

func (s *Service) request(ctx context.Context, kind jobs.Kind, id int64) error {
	if _, err := s.st.GetBookmark(ctx, id); err != nil {
		return err
	}
	return s.enqueueErr(ctx, kind, id)
}

// ReaderPage is one page of a bookmark's reader text. Offsets and lengths
// count runes. NextOffset is nil on the last page.
type ReaderPage struct {
	Bookmark    bookmarks.Bookmark
	Status      bookmarks.ReaderStatus
	Error       string
	Markdown    string
	Offset      int
	TotalChars  int
	NextOffset  *int
	WordCount   int
	ExtractedAt time.Time
}

// Reader returns up to maxChars runes of reader text starting at offset
// (maxChars 0 returns the rest). A page that does not reach the end is cut
// after the last paragraph break ("\n\n") in its window when there is one.
func (s *Service) Reader(ctx context.Context, id int64, offset, maxChars int) (ReaderPage, error) {
	if offset < 0 {
		return ReaderPage{}, invalid("offset", "must not be negative")
	}
	if maxChars < 0 {
		return ReaderPage{}, invalid("max_chars", "must not be negative")
	}
	b, err := s.st.GetBookmark(ctx, id)
	if err != nil {
		return ReaderPage{}, err
	}
	r, err := s.st.GetReaderText(ctx, id)
	if err != nil {
		return ReaderPage{}, err
	}
	text := []rune(r.Markdown)
	if offset > len(text) {
		return ReaderPage{}, invalid("offset", "is past the end of the text")
	}
	p := ReaderPage{
		Bookmark: b, Status: r.Status, Error: r.Error, Offset: offset,
		TotalChars: len(text), WordCount: r.WordCount, ExtractedAt: r.ExtractedAt,
	}
	rest := text[offset:]
	if maxChars == 0 || len(rest) <= maxChars {
		p.Markdown = string(rest)
		return p, nil
	}
	window := string(rest[:maxChars])
	cut := maxChars
	if i := strings.LastIndex(window, "\n\n"); i >= 0 {
		cut = utf8.RuneCountInString(window[:i+2])
	}
	p.Markdown = string(rest[:cut])
	next := offset + cut
	p.NextOffset = &next
	return p, nil
}

// ListArchives returns the archives of bookmark id, newest first as the store
// orders them. An unknown bookmark returns store.ErrNotFound.
func (s *Service) ListArchives(ctx context.Context, bookmarkID int64) ([]bookmarks.Archive, error) {
	if _, err := s.st.GetBookmark(ctx, bookmarkID); err != nil {
		return nil, err
	}
	return s.st.ListArchives(ctx, bookmarkID)
}
