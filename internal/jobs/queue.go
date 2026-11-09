// Package jobs is a small durable background job queue. It knows nothing about
// storage: persistence is supplied through the Store interface.
package jobs

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"
)

// Kind identifies the type of work a job does.
type Kind string

const (
	KindMetadata Kind = "metadata"
	KindCapture  Kind = "capture"
	KindExtract  Kind = "extract"
	KindWayback  Kind = "wayback"
)

// Job is a claimed unit of work. Attempts includes the current attempt.
type Job struct {
	ID, BookmarkID int64
	Kind           Kind
	Attempts       int
}

// Store persists jobs.
type Store interface {
	// EnqueueJob is a no-op if an identical (kind, bookmark) pending job exists.
	EnqueueJob(ctx context.Context, kind Kind, bookmarkID int64, runAt time.Time) error
	// ClaimJob moves the oldest pending job with run_at <= now to running and
	// increments its attempts.
	ClaimJob(ctx context.Context, now time.Time) (Job, bool, error)
	CompleteJob(ctx context.Context, id int64) error
	// RetryJob puts a job back to pending to run at runAt.
	RetryJob(ctx context.Context, id int64, runAt time.Time, msg string) error
	FailJob(ctx context.Context, id int64, msg string) error
	// ReleaseJob returns a claimed job to pending without consuming its attempt
	// (used when the queue is shutting down mid-job).
	ReleaseJob(ctx context.Context, id int64) error
	// ResetRunningJobs moves running jobs back to pending (startup recovery).
	ResetRunningJobs(ctx context.Context) error
}

// Handler runs a job. Return an error wrapping ErrPermanent to skip retries.
type Handler func(ctx context.Context, j Job) error

// ErrPermanent marks a handler error that must not be retried.
var ErrPermanent = errors.New("permanent")

// maxBackoffShift caps the backoff exponent so the shift cannot overflow.
const maxBackoffShift = 20

// Options configures a Queue; zero values take the defaults.
type Options struct {
	Workers      int           // 2
	MaxAttempts  int           // 3
	BaseBackoff  time.Duration // 30s
	PollInterval time.Duration // 2s
	Now          func() time.Time
	Logger       *log.Logger // default: the standard logger
}

// Queue runs jobs from a Store on a pool of workers.
type Queue struct {
	st       Store
	handlers map[Kind]Handler
	o        Options
	wake     chan struct{}
}

// New builds a Queue.
func New(st Store, handlers map[Kind]Handler, o Options) *Queue {
	if o.Workers <= 0 {
		o.Workers = 2
	}
	if o.MaxAttempts <= 0 {
		o.MaxAttempts = 3
	}
	if o.BaseBackoff <= 0 {
		o.BaseBackoff = 30 * time.Second
	}
	if o.PollInterval <= 0 {
		o.PollInterval = 2 * time.Second
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Logger == nil {
		o.Logger = log.Default()
	}
	return &Queue{st: st, handlers: handlers, o: o, wake: make(chan struct{}, o.Workers)}
}

// Enqueue stores a job to run now and wakes a worker.
func (q *Queue) Enqueue(ctx context.Context, kind Kind, bookmarkID int64) error {
	if err := q.st.EnqueueJob(ctx, kind, bookmarkID, q.o.Now()); err != nil {
		return err
	}
	select {
	case q.wake <- struct{}{}:
	default:
	}
	return nil
}

// Run recovers jobs left running by a previous process, then processes jobs
// until ctx is done and every worker has finished its current job.
func (q *Queue) Run(ctx context.Context) error {
	if err := q.st.ResetRunningJobs(ctx); err != nil {
		return fmt.Errorf("reset running jobs: %w", err)
	}
	var wg sync.WaitGroup
	for i := 0; i < q.o.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			q.worker(ctx)
		}()
	}
	wg.Wait()
	return nil
}

func (q *Queue) worker(ctx context.Context) {
	for ctx.Err() == nil {
		j, ok, err := q.st.ClaimJob(ctx, q.o.Now())
		if err != nil {
			if ctx.Err() == nil {
				q.o.Logger.Printf("jobs: claim: %v", err)
			}
			q.sleep(ctx, false)
			continue
		}
		if !ok {
			q.sleep(ctx, true)
			continue
		}
		q.process(ctx, j)
	}
}

// sleep waits for a wake-up (if allowed), the poll interval, or ctx.Done.
func (q *Queue) sleep(ctx context.Context, wakeable bool) {
	t := time.NewTimer(q.o.PollInterval)
	defer t.Stop()
	var wake <-chan struct{}
	if wakeable {
		wake = q.wake
	}
	select {
	case <-ctx.Done():
	case <-wake:
	case <-t.C:
	}
}

func (q *Queue) process(ctx context.Context, j Job) {
	// Bookkeeping must survive shutdown so a finished job is recorded.
	sctx := context.WithoutCancel(ctx)
	h, ok := q.handlers[j.Kind]
	if !ok {
		q.record(q.st.FailJob(sctx, j.ID, fmt.Sprintf("no handler for kind %q", j.Kind)), j)
		return
	}
	err := q.call(ctx, h, j)
	switch {
	case err != nil && ctx.Err() != nil:
		// Shutting down: the failure is most likely the cancellation, so give the
		// attempt back instead of retrying or failing the job.
		q.record(q.st.ReleaseJob(sctx, j.ID), j)
	case err == nil:
		q.record(q.st.CompleteJob(sctx, j.ID), j)
	case errors.Is(err, ErrPermanent) || j.Attempts >= q.o.MaxAttempts:
		q.o.Logger.Printf("jobs: %s job %d (bookmark %d) failed: %v", j.Kind, j.ID, j.BookmarkID, err)
		q.record(q.st.FailJob(sctx, j.ID, err.Error()), j)
	default:
		delay := q.o.BaseBackoff << min(j.Attempts-1, maxBackoffShift)
		q.o.Logger.Printf("jobs: %s job %d (bookmark %d) attempt %d failed, retrying in %s: %v", j.Kind, j.ID, j.BookmarkID, j.Attempts, delay, err)
		q.record(q.st.RetryJob(sctx, j.ID, q.o.Now().Add(delay), err.Error()), j)
	}
}

func (q *Queue) record(err error, j Job) {
	if err != nil {
		q.o.Logger.Printf("jobs: update job %d: %v", j.ID, err)
	}
}

func (q *Queue) call(ctx context.Context, h Handler, j Job) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return h(ctx, j)
}
