package store

import (
	"context"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/danielfrg/bookmarks/internal/jobs"
)

func jobBookmark(t *testing.T, s *SQLite, url string) int64 {
	t.Helper()
	b := sample(url)
	if err := s.CreateBookmark(context.Background(), b); err != nil {
		t.Fatal(err)
	}
	return b.ID
}

func jobRow(t *testing.T, s *SQLite, id int64) (status string, attempts int, lastErr, runAt string) {
	t.Helper()
	err := s.db.QueryRow("SELECT status, attempts, last_error, run_at FROM jobs WHERE id = ?", id).Scan(&status, &attempts, &lastErr, &runAt)
	if err != nil {
		t.Fatal(err)
	}
	return
}

func countJobs(t *testing.T, s *SQLite, where string, args ...any) int {
	t.Helper()
	var n int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM jobs WHERE "+where, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

var _ jobs.Store = (*SQLite)(nil)

func TestEnqueueJobDedupesPending(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	bid := jobBookmark(t, s, "https://a.example/")
	at := time.Now()
	for i := 0; i < 3; i++ {
		if err := s.EnqueueJob(ctx, jobs.KindMetadata, bid, at); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.EnqueueJob(ctx, jobs.KindCapture, bid, at); err != nil {
		t.Fatal(err)
	}
	if n := countJobs(t, s, "bookmark_id = ?", bid); n != 2 {
		t.Fatalf("jobs = %d, want 2", n)
	}
}

func TestEnqueueJobAllowedWhileRunning(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	bid := jobBookmark(t, s, "https://a.example/")
	now := time.Now()
	s.EnqueueJob(ctx, jobs.KindExtract, bid, now)
	if _, ok, err := s.ClaimJob(ctx, now.Add(time.Second)); err != nil || !ok {
		t.Fatalf("claim: %v %v", ok, err)
	}
	if err := s.EnqueueJob(ctx, jobs.KindExtract, bid, now); err != nil {
		t.Fatal(err)
	}
	if n := countJobs(t, s, "status = 'pending'"); n != 1 {
		t.Fatalf("pending = %d, want 1", n)
	}
}

func TestClaimJobOrderAndDueness(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	b1 := jobBookmark(t, s, "https://a.example/")
	b2 := jobBookmark(t, s, "https://b.example/")
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s.EnqueueJob(ctx, jobs.KindMetadata, b2, base.Add(time.Minute))
	s.EnqueueJob(ctx, jobs.KindMetadata, b1, base)
	s.EnqueueJob(ctx, jobs.KindCapture, b1, base.Add(time.Hour))

	j, ok, err := s.ClaimJob(ctx, base.Add(2*time.Minute))
	if err != nil || !ok {
		t.Fatalf("claim: %v %v", ok, err)
	}
	if j.BookmarkID != b1 || j.Kind != jobs.KindMetadata || j.Attempts != 1 {
		t.Fatalf("first claim = %+v", j)
	}
	if st, att, _, _ := jobRow(t, s, j.ID); st != "running" || att != 1 {
		t.Fatalf("row = %s %d", st, att)
	}
	j, ok, _ = s.ClaimJob(ctx, base.Add(2*time.Minute))
	if !ok || j.BookmarkID != b2 {
		t.Fatalf("second claim = %+v %v", j, ok)
	}
	if _, ok, _ := s.ClaimJob(ctx, base.Add(2*time.Minute)); ok {
		t.Fatal("future job claimed")
	}
}

func TestJobLifecycle(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	bid := jobBookmark(t, s, "https://a.example/")
	now := time.Now().UTC()
	s.EnqueueJob(ctx, jobs.KindMetadata, bid, now)
	j, _, _ := s.ClaimJob(ctx, now)

	retryAt := now.Add(time.Minute)
	if err := s.RetryJob(ctx, j.ID, retryAt, "boom"); err != nil {
		t.Fatal(err)
	}
	st, att, msg, runAt := jobRow(t, s, j.ID)
	if st != "pending" || att != 1 || msg != "boom" || runAt != formatTime(retryAt) {
		t.Fatalf("after retry: %s %d %q %s", st, att, msg, runAt)
	}
	if _, ok, _ := s.ClaimJob(ctx, now); ok {
		t.Fatal("claimed before run_at")
	}
	j, ok, _ := s.ClaimJob(ctx, retryAt)
	if !ok || j.Attempts != 2 {
		t.Fatalf("reclaim = %+v %v", j, ok)
	}
	if err := s.FailJob(ctx, j.ID, "dead"); err != nil {
		t.Fatal(err)
	}
	if st, _, msg, _ := jobRow(t, s, j.ID); st != "failed" || msg != "dead" {
		t.Fatalf("after fail: %s %q", st, msg)
	}

	s.EnqueueJob(ctx, jobs.KindCapture, bid, now)
	j, _, _ = s.ClaimJob(ctx, now)
	if err := s.CompleteJob(ctx, j.ID); err != nil {
		t.Fatal(err)
	}
	if st, _, _, _ := jobRow(t, s, j.ID); st != "done" {
		t.Fatalf("after complete: %s", st)
	}
}

func TestResetRunningJobs(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	bid := jobBookmark(t, s, "https://a.example/")
	now := time.Now()
	s.EnqueueJob(ctx, jobs.KindMetadata, bid, now)
	j, _, _ := s.ClaimJob(ctx, now)
	if err := s.ResetRunningJobs(ctx); err != nil {
		t.Fatal(err)
	}
	if st, _, _, _ := jobRow(t, s, j.ID); st != "pending" {
		t.Fatalf("status = %s", st)
	}
}

func TestClaimJobPrunesOldDone(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	bid := jobBookmark(t, s, "https://a.example/")
	now := time.Now().UTC()
	old, recent := formatTime(now.Add(-8*24*time.Hour)), formatTime(now.Add(-6*24*time.Hour))
	for _, ts := range []string{old, recent} {
		if _, err := s.db.Exec(`INSERT INTO jobs (kind, bookmark_id, status, run_at, created_at, updated_at) VALUES ('metadata', ?, 'done', ?, ?, ?)`, bid, ts, ts, ts); err != nil {
			t.Fatal(err)
		}
	}
	if _, ok, err := s.ClaimJob(ctx, now); err != nil || ok {
		t.Fatalf("claim: %v %v", ok, err)
	}
	if n := countJobs(t, s, "status = 'done'"); n != 1 {
		t.Fatalf("done = %d, want 1", n)
	}
}

func TestJobErrorTruncatedAndCascade(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	bid := jobBookmark(t, s, "https://a.example/")
	now := time.Now()
	s.EnqueueJob(ctx, jobs.KindMetadata, bid, now)
	j, _, _ := s.ClaimJob(ctx, now)
	s.FailJob(ctx, j.ID, strings.Repeat("x", 5000))
	if _, _, msg, _ := jobRow(t, s, j.ID); len(msg) != 1000 {
		t.Fatalf("len = %d", len(msg))
	}
	if err := s.DeleteBookmark(ctx, bid); err != nil {
		t.Fatal(err)
	}
	if n := countJobs(t, s, "1=1"); n != 0 {
		t.Fatalf("jobs after cascade = %d", n)
	}
}

func TestReleaseJobRestoresAttempt(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	bid := jobBookmark(t, s, "https://a.example/")
	now := time.Now()
	if err := s.EnqueueJob(ctx, jobs.KindMetadata, bid, now); err != nil {
		t.Fatal(err)
	}
	j, ok, err := s.ClaimJob(ctx, now)
	if err != nil || !ok {
		t.Fatalf("claim: %v %v", ok, err)
	}
	if err := s.ReleaseJob(ctx, j.ID); err != nil {
		t.Fatal(err)
	}
	if st, att, _, _ := jobRow(t, s, j.ID); st != "pending" || att != 0 {
		t.Fatalf("row = %s %d", st, att)
	}
	if j2, ok, _ := s.ClaimJob(ctx, now); !ok || j2.Attempts != 1 {
		t.Fatalf("reclaim = %+v %v", j2, ok)
	}
}

func TestJobErrorTruncatedOnRuneBoundary(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	bid := jobBookmark(t, s, "https://a.example/")
	now := time.Now()
	if err := s.EnqueueJob(ctx, jobs.KindMetadata, bid, now); err != nil {
		t.Fatal(err)
	}
	j, _, _ := s.ClaimJob(ctx, now)
	if err := s.FailJob(ctx, j.ID, "x"+strings.Repeat("é", 600)); err != nil {
		t.Fatal(err)
	}
	_, _, msg, _ := jobRow(t, s, j.ID)
	if !utf8.ValidString(msg) || len(msg) > 1000 || len(msg) < 998 {
		t.Fatalf("len = %d valid = %v", len(msg), utf8.ValidString(msg))
	}
}
