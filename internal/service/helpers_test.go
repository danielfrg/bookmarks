package service

import (
	"context"
	"io"
	"log"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/danielfrg/bookmarks/internal/capture"
	"github.com/danielfrg/bookmarks/internal/fetch"
	"github.com/danielfrg/bookmarks/internal/jobs"
	"github.com/danielfrg/bookmarks/internal/store"
)

type enqueued struct {
	Kind       jobs.Kind
	BookmarkID int64
}

type fakeQueue struct {
	mu   sync.Mutex
	jobs []enqueued
}

func (q *fakeQueue) Enqueue(_ context.Context, kind jobs.Kind, id int64) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.jobs = append(q.jobs, enqueued{kind, id})
	return nil
}

func (q *fakeQueue) kinds(id int64) []jobs.Kind {
	q.mu.Lock()
	defer q.mu.Unlock()
	var out []jobs.Kind
	for _, j := range q.jobs {
		if j.BookmarkID == id {
			out = append(out, j.Kind)
		}
	}
	return out
}

func (q *fakeQueue) reset() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.jobs = nil
}

type fakeCapturer struct {
	res capture.Result
	err error
}

func (c *fakeCapturer) Capture(context.Context, string) (capture.Result, error) {
	return c.res, c.err
}

type env struct {
	svc *Service
	st  *store.SQLite
	q   *fakeQueue
	cap *fakeCapturer
	dir string
}

var fixedNow = time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)

func newEnv(t *testing.T) *env {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "bookmarks.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	q := &fakeQueue{}
	c := &fakeCapturer{}
	fc := fetch.NewClient(fetch.Options{AllowPrivate: true})
	svc := New(st, fc, c, q, Options{
		DataDir:        dir,
		MaxUploadBytes: 1 << 20,
		Now:            func() time.Time { return fixedNow },
		Logger:         log.New(io.Discard, "", 0),
	})
	return &env{svc: svc, st: st, q: q, cap: c, dir: dir}
}

func ptr[T any](v T) *T { return &v }
