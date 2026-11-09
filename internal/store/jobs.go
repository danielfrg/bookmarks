package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
	"unicode/utf8"

	"github.com/danielfrg/bookmarks/internal/jobs"
)

const (
	doneJobRetention = 7 * 24 * time.Hour
	maxJobErrorLen   = 1000
)

func truncateJobError(s string) string {
	if len(s) <= maxJobErrorLen {
		return s
	}
	cut := maxJobErrorLen
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}

// EnqueueJob adds a pending job unless an identical pending one exists. A
// running job does not block a new pending one.
func (s *SQLite) EnqueueJob(ctx context.Context, kind jobs.Kind, bookmarkID int64, runAt time.Time) error {
	ts := formatTime(now())
	_, err := s.db.ExecContext(ctx, `
INSERT INTO jobs (kind, bookmark_id, status, run_at, created_at, updated_at)
SELECT ?, ?, 'pending', ?, ?, ?
WHERE NOT EXISTS (SELECT 1 FROM jobs WHERE kind = ? AND bookmark_id = ? AND status = 'pending')`,
		string(kind), bookmarkID, formatTime(runAt), ts, ts, string(kind), bookmarkID)
	return err
}

// ClaimJob atomically moves the oldest due pending job to running. It also
// prunes done jobs older than seven days.
func (s *SQLite) ClaimJob(ctx context.Context, at time.Time) (jobs.Job, bool, error) {
	if _, err := s.db.ExecContext(ctx, "DELETE FROM jobs WHERE status = 'done' AND updated_at < ?", formatTime(at.Add(-doneJobRetention))); err != nil {
		return jobs.Job{}, false, err
	}
	var j jobs.Job
	var kind string
	err := s.db.QueryRowContext(ctx, `
UPDATE jobs SET status = 'running', attempts = attempts + 1, updated_at = ?
WHERE id = (SELECT id FROM jobs WHERE status = 'pending' AND run_at <= ? ORDER BY run_at, id LIMIT 1)
RETURNING id, kind, bookmark_id, attempts`, formatTime(now()), formatTime(at)).Scan(&j.ID, &kind, &j.BookmarkID, &j.Attempts)
	if errors.Is(err, sql.ErrNoRows) {
		return jobs.Job{}, false, nil
	}
	if err != nil {
		return jobs.Job{}, false, err
	}
	j.Kind = jobs.Kind(kind)
	return j, true, nil
}

func (s *SQLite) CompleteJob(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, "UPDATE jobs SET status = 'done', updated_at = ? WHERE id = ?", formatTime(now()), id)
	return err
}

func (s *SQLite) RetryJob(ctx context.Context, id int64, runAt time.Time, msg string) error {
	_, err := s.db.ExecContext(ctx, "UPDATE jobs SET status = 'pending', run_at = ?, last_error = ?, updated_at = ? WHERE id = ?",
		formatTime(runAt), truncateJobError(msg), formatTime(now()), id)
	return err
}

func (s *SQLite) FailJob(ctx context.Context, id int64, msg string) error {
	_, err := s.db.ExecContext(ctx, "UPDATE jobs SET status = 'failed', last_error = ?, updated_at = ? WHERE id = ?",
		truncateJobError(msg), formatTime(now()), id)
	return err
}

func (s *SQLite) ResetRunningJobs(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, "UPDATE jobs SET status = 'pending', updated_at = ? WHERE status = 'running'", formatTime(now()))
	return err
}

// ReleaseJob returns a running job to pending and gives back its attempt.
func (s *SQLite) ReleaseJob(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, "UPDATE jobs SET status = 'pending', attempts = MAX(attempts - 1, 0), updated_at = ? WHERE id = ? AND status = 'running'", formatTime(now()), id)
	return err
}
