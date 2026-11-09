package jobs

import (
	"context"
	"fmt"
	"io"
	"log"
	"sync"
	"testing"
	"time"
)

type row struct {
	Job
	status  string
	runAt   time.Time
	lastErr string
}

type fakeStore struct {
	mu   sync.Mutex
	next int64
	rows []*row
}

func (f *fakeStore) EnqueueJob(_ context.Context, kind Kind, bid int64, runAt time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.rows {
		if r.Kind == kind && r.BookmarkID == bid && r.status == "pending" {
			return nil
		}
	}
	f.next++
	f.rows = append(f.rows, &row{Job: Job{ID: f.next, Kind: kind, BookmarkID: bid}, status: "pending", runAt: runAt})
	return nil
}

func (f *fakeStore) ClaimJob(_ context.Context, now time.Time) (Job, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.rows {
		if r.status == "pending" && !r.runAt.After(now) {
			r.status = "running"
			r.Attempts++
			return r.Job, true, nil
		}
	}
	return Job{}, false, nil
}

func (f *fakeStore) set(id int64, fn func(*row)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.rows {
		if r.ID == id {
			fn(r)
		}
	}
}

func (f *fakeStore) CompleteJob(_ context.Context, id int64) error {
	f.set(id, func(r *row) { r.status = "done" })
	return nil
}
func (f *fakeStore) RetryJob(_ context.Context, id int64, runAt time.Time, msg string) error {
	f.set(id, func(r *row) { r.status, r.runAt, r.lastErr = "pending", runAt, msg })
	return nil
}
func (f *fakeStore) FailJob(_ context.Context, id int64, msg string) error {
	f.set(id, func(r *row) { r.status, r.lastErr = "failed", msg })
	return nil
}
func (f *fakeStore) ReleaseJob(_ context.Context, id int64) error {
	f.set(id, func(r *row) { r.status = "pending"; r.Attempts-- })
	return nil
}
func (f *fakeStore) ResetRunningJobs(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.rows {
		if r.status == "running" {
			r.status = "pending"
		}
	}
	return nil
}

func (f *fakeStore) get(i int) row {
	f.mu.Lock()
	defer f.mu.Unlock()
	return *f.rows[i]
}

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) Advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func newClock() *clock { return &clock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)} }

func start(t *testing.T, q *Queue) (stop func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- q.Run(ctx) }()
	var once sync.Once
	return func() {
		once.Do(func() {
			cancel()
			select {
			case err := <-done:
				if err != nil {
					t.Errorf("Run = %v", err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("Run did not return")
			}
		})
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met")
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func opts(c *clock) Options {
	return Options{Workers: 2, MaxAttempts: 3, BaseBackoff: time.Minute, PollInterval: 5 * time.Millisecond, Now: c.Now, Logger: log.New(io.Discard, "", 0)}
}

func TestEnqueueDedupes(t *testing.T) {
	st := &fakeStore{}
	q := New(st, nil, opts(newClock()))
	for i := 0; i < 3; i++ {
		if err := q.Enqueue(context.Background(), KindMetadata, 1); err != nil {
			t.Fatal(err)
		}
	}
	if len(st.rows) != 1 {
		t.Fatalf("rows = %d", len(st.rows))
	}
}

func TestQueueRunsHandler(t *testing.T) {
	st, c := &fakeStore{}, newClock()
	calls := make(chan Job, 4)
	q := New(st, map[Kind]Handler{KindMetadata: func(_ context.Context, j Job) error { calls <- j; return nil }}, opts(c))
	stop := start(t, q)
	defer stop()
	if err := q.Enqueue(context.Background(), KindMetadata, 7); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return st.get(0).status == "done" })
	stop()
	if len(calls) != 1 {
		t.Fatalf("handler calls = %d", len(calls))
	}
	if j := <-calls; j.BookmarkID != 7 || j.Attempts != 1 {
		t.Fatalf("job = %+v", j)
	}
}

func TestQueueRetriesThenFails(t *testing.T) {
	st, c := &fakeStore{}, newClock()
	var mu sync.Mutex
	n := 0
	q := New(st, map[Kind]Handler{KindCapture: func(context.Context, Job) error {
		mu.Lock()
		defer mu.Unlock()
		n++
		return fmt.Errorf("attempt %d failed", n)
	}}, opts(c))
	stop := start(t, q)
	defer stop()
	start0 := c.Now()
	if err := q.Enqueue(context.Background(), KindCapture, 1); err != nil {
		t.Fatal(err)
	}

	waitFor(t, func() bool { r := st.get(0); return r.status == "pending" && r.Attempts == 1 })
	if r := st.get(0); !r.runAt.Equal(start0.Add(time.Minute)) {
		t.Fatalf("first backoff runAt = %v", r.runAt)
	}
	time.Sleep(30 * time.Millisecond) // not due: must not run again
	mu.Lock()
	if n != 1 {
		t.Fatalf("ran %d times before backoff elapsed", n)
	}
	mu.Unlock()

	c.Advance(time.Minute)
	waitFor(t, func() bool { r := st.get(0); return r.status == "pending" && r.Attempts == 2 })
	if r := st.get(0); !r.runAt.Equal(c.Now().Add(2 * time.Minute)) {
		t.Fatalf("second backoff runAt = %v", r.runAt)
	}
	c.Advance(2 * time.Minute)
	waitFor(t, func() bool { return st.get(0).status == "failed" })
	r := st.get(0)
	if r.Attempts != 3 || r.lastErr != "attempt 3 failed" {
		t.Fatalf("row = %+v", r)
	}
}

func TestPermanentErrorNoRetry(t *testing.T) {
	st, c := &fakeStore{}, newClock()
	q := New(st, map[Kind]Handler{KindWayback: func(context.Context, Job) error {
		return fmt.Errorf("bad url: %w", ErrPermanent)
	}}, opts(c))
	stop := start(t, q)
	defer stop()
	if err := q.Enqueue(context.Background(), KindWayback, 1); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return st.get(0).status == "failed" })
	if r := st.get(0); r.Attempts != 1 || r.lastErr == "" {
		t.Fatalf("row = %+v", r)
	}
}

func TestUnknownKindFails(t *testing.T) {
	st := &fakeStore{}
	q := New(st, map[Kind]Handler{}, opts(newClock()))
	stop := start(t, q)
	defer stop()
	if err := q.Enqueue(context.Background(), KindExtract, 1); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return st.get(0).status == "failed" })
}

func TestPanicRecovered(t *testing.T) {
	st, c := &fakeStore{}, newClock()
	q := New(st, map[Kind]Handler{KindExtract: func(context.Context, Job) error { panic("kaboom") }}, opts(c))
	stop := start(t, q)
	defer stop()
	if err := q.Enqueue(context.Background(), KindExtract, 1); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { r := st.get(0); return r.status == "pending" && r.Attempts == 1 })
	if r := st.get(0); r.lastErr != "panic: kaboom" {
		t.Fatalf("lastErr = %q", r.lastErr)
	}
}

func TestResetRunningOnStart(t *testing.T) {
	st, c := &fakeStore{}, newClock()
	st.rows = []*row{{Job: Job{ID: 1, Kind: KindMetadata, BookmarkID: 1, Attempts: 1}, status: "running", runAt: c.Now()}}
	st.next = 1
	done := make(chan struct{})
	q := New(st, map[Kind]Handler{KindMetadata: func(context.Context, Job) error { close(done); return nil }}, opts(c))
	stop := start(t, q)
	defer stop()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("running job not handled after start")
	}
}

func TestRunWaitsForInFlightJob(t *testing.T) {
	st, c := &fakeStore{}, newClock()
	started, release := make(chan struct{}), make(chan struct{})
	q := New(st, map[Kind]Handler{KindMetadata: func(context.Context, Job) error {
		close(started)
		<-release
		return nil
	}}, opts(c))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- q.Run(ctx) }()
	if err := q.Enqueue(context.Background(), KindMetadata, 1); err != nil {
		t.Fatal(err)
	}
	<-started
	cancel()
	select {
	case <-done:
		t.Fatal("Run returned while handler in flight")
	case <-time.After(30 * time.Millisecond):
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if st.get(0).status != "done" {
		t.Fatalf("status = %s", st.get(0).status)
	}
}

func TestShutdownReleasesJobInsteadOfFailing(t *testing.T) {
	st, c := &fakeStore{}, newClock()
	started := make(chan struct{})
	o := opts(c)
	o.MaxAttempts = 1
	q := New(st, map[Kind]Handler{KindMetadata: func(ctx context.Context, _ Job) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}}, o)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- q.Run(ctx) }()
	if err := q.Enqueue(context.Background(), KindMetadata, 1); err != nil {
		t.Fatal(err)
	}
	<-started
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if r := st.get(0); r.status != "pending" || r.Attempts != 0 {
		t.Fatalf("row = %+v", r)
	}
}
