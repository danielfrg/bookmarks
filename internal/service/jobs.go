package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/danielfrg/bookmarks/internal/archive"
	"github.com/danielfrg/bookmarks/internal/bookmarks"
	"github.com/danielfrg/bookmarks/internal/capture"
	"github.com/danielfrg/bookmarks/internal/fetch"
	"github.com/danielfrg/bookmarks/internal/jobs"
	"github.com/danielfrg/bookmarks/internal/reader"
	"github.com/danielfrg/bookmarks/internal/store"
)

// JobHandlers returns the background job handlers for the queue.
func (s *Service) JobHandlers() map[jobs.Kind]jobs.Handler {
	return map[jobs.Kind]jobs.Handler{
		jobs.KindMetadata: s.metadataJob,
		jobs.KindCapture:  s.captureJob,
		jobs.KindExtract:  s.extractJob,
		jobs.KindWayback:  s.waybackJob,
	}
}

func permanent(err error) error { return fmt.Errorf("%w: %w", jobs.ErrPermanent, err) }

// jobBookmark loads the job's bookmark. ok is false (with a nil error) when it
// was deleted, which completes the job.
func (s *Service) jobBookmark(ctx context.Context, id int64) (b bookmarks.Bookmark, ok bool, err error) {
	b, err = s.st.GetBookmark(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return b, false, nil
	}
	return b, err == nil, err
}

// patchBookmark reloads bookmark id, applies fn and saves it without touching
// DateModified (background jobs are not user edits).
func (s *Service) patchBookmark(ctx context.Context, id int64, fn func(*bookmarks.Bookmark)) error {
	b, ok, err := s.jobBookmark(ctx, id)
	if !ok {
		return err
	}
	fn(&b)
	return s.st.UpdateBookmark(ctx, b)
}

// metadataJob fills an empty title and description from the page, then
// downloads the favicon and preview image when enabled.
func (s *Service) metadataJob(ctx context.Context, j jobs.Job) error {
	b, ok, err := s.jobBookmark(ctx, j.BookmarkID)
	if !ok {
		return err
	}
	md, err := s.fc.Metadata(ctx, b.URL)
	if errors.Is(err, fetch.ErrBlockedAddress) {
		return permanent(err)
	}
	if err != nil {
		return err
	}
	if err := s.patchBookmark(ctx, b.ID, func(b *bookmarks.Bookmark) {
		if b.Title == "" {
			b.Title = md.Title
		}
		if b.Description == "" {
			b.Description = md.Description
		}
	}); err != nil {
		return err
	}

	settings, err := s.st.GetSettings(ctx)
	if err != nil {
		return err
	}
	var favicon, preview string
	if settings.FaviconsEnabled && s.o.FaviconProvider != "" {
		if favicon, err = s.fc.DownloadFavicon(ctx, s.o.FaviconProvider, b.URL, s.faviconDir()); err != nil {
			s.log.Printf("service: favicon for bookmark %d: %v", b.ID, err)
		}
	}
	if settings.PreviewsEnabled && md.PreviewImageURL != "" {
		if preview, err = s.fc.DownloadPreview(ctx, md.PreviewImageURL, s.previewDir()); err != nil {
			s.log.Printf("service: preview for bookmark %d: %v", b.ID, err)
		}
	}
	if favicon == "" && preview == "" {
		return nil
	}
	var oldPreview string
	if err := s.patchBookmark(ctx, b.ID, func(b *bookmarks.Bookmark) {
		if favicon != "" {
			b.FaviconFile = favicon
		}
		if preview != "" && preview != b.PreviewFile {
			oldPreview, b.PreviewFile = b.PreviewFile, preview
		}
	}); err != nil {
		return err
	}
	s.removePreviewIfUnused(ctx, oldPreview)
	return nil
}

// captureJob saves a basic capture of the page as a server archive.
func (s *Service) captureJob(ctx context.Context, j jobs.Job) error {
	b, ok, err := s.jobBookmark(ctx, j.BookmarkID)
	if !ok {
		return err
	}
	if s.cap == nil {
		return permanent(errors.New("no capturer configured"))
	}
	res, err := s.cap.Capture(ctx, b.URL)
	if err != nil {
		var he *capture.HTTPError
		if errors.As(err, &he) && he.Status >= 400 && he.Status < 500 ||
			errors.Is(err, fetch.ErrBlockedAddress) ||
			errors.Is(err, capture.ErrUnsupported) ||
			errors.Is(err, capture.ErrTooLarge) {
			return permanent(err)
		}
		return err
	}
	_, err = s.AddArchive(ctx, b.ID, ArchiveInput{
		Source:      bookmarks.SourceServer,
		ContentType: res.ContentType,
		DisplayName: "Basic capture",
		Body:        bytes.NewReader(res.Body),
	})
	var ve *ValidationError
	switch {
	case errors.Is(err, store.ErrNotFound):
		return nil // deleted while capturing
	case errors.Is(err, archive.ErrTooLarge) || errors.As(err, &ve):
		return permanent(err)
	}
	return err
}

// extractJob extracts reader text from the newest readable archive. Content
// that cannot be extracted is recorded as failed and not retried.
func (s *Service) extractJob(ctx context.Context, j jobs.Job) error {
	b, ok, err := s.jobBookmark(ctx, j.BookmarkID)
	if !ok {
		return err
	}
	a, err := s.st.LatestReadableArchive(ctx, b.ID)
	if errors.Is(err, store.ErrNotFound) {
		return s.st.PutReaderText(ctx, bookmarks.ReaderText{
			BookmarkID: b.ID, Status: bookmarks.ReaderNone, ExtractedAt: s.now(),
		})
	}
	if err != nil {
		return err
	}
	failed := func(msg string) error {
		return s.st.PutReaderText(ctx, bookmarks.ReaderText{
			BookmarkID: b.ID, ArchiveID: a.ID, Status: bookmarks.ReaderFailed, Error: msg, ExtractedAt: s.now(),
		})
	}
	rc, err := s.files.Open(a.FileName)
	if err != nil {
		s.log.Printf("service: open archive %d: %v", a.ID, err)
		return failed("archive file could not be read")
	}
	defer rc.Close()
	res, err := reader.Extract(ctx, a.ContentType, rc, b.URL)
	switch {
	case err == nil:
	case errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil:
		return err
	case errors.Is(err, reader.ErrNoContent) || errors.Is(err, reader.ErrUnsupported):
		return failed(err.Error())
	default:
		s.log.Printf("service: extract archive %d: %v", a.ID, err)
		return failed("text could not be extracted")
	}
	return s.st.PutReaderText(ctx, bookmarks.ReaderText{
		BookmarkID: b.ID, ArchiveID: a.ID, Status: bookmarks.ReaderOK,
		Title: res.Title, Markdown: res.Markdown, WordCount: res.WordCount, ExtractedAt: s.now(),
	})
}

// waybackJob asks the Wayback Machine to save the page and records the
// snapshot URL (linkding's fallback when the final URL is not a snapshot).
func (s *Service) waybackJob(ctx context.Context, j jobs.Job) error {
	b, ok, err := s.jobBookmark(ctx, j.BookmarkID)
	if !ok {
		return err
	}
	resp, err := s.fc.Get(ctx, s.waybackBase+"/save/"+b.URL, 1<<20)
	if err != nil {
		return err
	}
	if resp.Status < 200 || resp.Status > 299 {
		return fmt.Errorf("wayback save returned status %d", resp.Status)
	}
	snapshot := resp.FinalURL
	if !strings.Contains(snapshot, "/web/") {
		snapshot = s.waybackBase + "/web/" + b.DateAdded.UTC().Format("20060102150405") + "/" + b.URL
	}
	return s.patchBookmark(ctx, b.ID, func(b *bookmarks.Bookmark) { b.WebArchiveURL = snapshot })
}
