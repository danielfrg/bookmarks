package service

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/danielfrg/bookmarks/internal/jobs"
	"github.com/danielfrg/bookmarks/internal/store"
)

func TestBulkActions(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	var ids []int64
	for i := range 3 {
		b, _, err := e.svc.Save(ctx, SaveInput{URL: fmt.Sprintf("https://%d.example", i), Tags: &[]string{"keep"}})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, b.ID)
	}

	n, err := e.svc.Bulk(ctx, BulkInput{IDs: []int64{ids[0], ids[1], 999}, Action: "tag", Tags: []string{"New Two"}})
	if err != nil || n != 2 {
		t.Fatalf("tag = %d, %v", n, err)
	}
	got, _ := e.svc.Get(ctx, ids[0])
	if !reflect.DeepEqual(got.Tags, []string{"keep", "new", "two"}) {
		t.Fatalf("tags = %v", got.Tags)
	}

	n, err = e.svc.Bulk(ctx, BulkInput{Query: &SearchInput{Q: "#new"}, Action: "unread"})
	if err != nil || n != 2 {
		t.Fatalf("unread by query = %d, %v", n, err)
	}
	res, _ := e.svc.Search(ctx, SearchInput{Unread: "no"})
	if res.Total != 1 || res.Hits[0].ID != ids[2] {
		t.Fatalf("read = %+v", res)
	}

	n, err = e.svc.Bulk(ctx, BulkInput{IDs: ids, Action: "untag", Tags: []string{"NEW"}})
	if err != nil || n != 3 {
		t.Fatalf("untag = %d, %v", n, err)
	}
	got, _ = e.svc.Get(ctx, ids[1])
	if !reflect.DeepEqual(got.Tags, []string{"keep", "two"}) {
		t.Fatalf("bookmark = %+v", got)
	}

	if n, err = e.svc.Bulk(ctx, BulkInput{IDs: ids, Action: "unread"}); err != nil || n != 3 {
		t.Fatalf("unread = %d, %v", n, err)
	}
	got, _ = e.svc.Get(ctx, ids[2])
	if !got.Unread {
		t.Fatal("unread not applied")
	}

	e.q.reset()
	if n, err = e.svc.Bulk(ctx, BulkInput{IDs: ids[:1], Action: "refresh_metadata"}); err != nil || n != 1 {
		t.Fatalf("refresh = %d, %v", n, err)
	}
	if n, err = e.svc.Bulk(ctx, BulkInput{IDs: ids[:1], Action: "capture"}); err != nil || n != 1 {
		t.Fatal(err)
	}
	if n, err = e.svc.Bulk(ctx, BulkInput{IDs: ids[:1], Action: "extract"}); err != nil || n != 1 {
		t.Fatal(err)
	}
	if got, want := e.q.kinds(ids[0]), []jobs.Kind{jobs.KindMetadata, jobs.KindCapture, jobs.KindExtract}; !reflect.DeepEqual(got, want) {
		t.Fatalf("jobs = %v", got)
	}

	if n, err = e.svc.Bulk(ctx, BulkInput{Query: &SearchInput{Unread: "yes"}, Action: "delete"}); err != nil || n != 3 {
		t.Fatalf("delete = %d, %v", n, err)
	}
	if _, err := e.svc.Get(ctx, ids[0]); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestBulkQueryPagesThroughAll(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	for i := range 503 {
		if _, _, err := e.svc.Save(ctx, SaveInput{URL: fmt.Sprintf("https://%d.example", i), Capture: CaptureNone}); err != nil {
			t.Fatal(err)
		}
	}
	n, err := e.svc.Bulk(ctx, BulkInput{Query: &SearchInput{}, Action: "read"})
	if err != nil || n != 503 {
		t.Fatalf("read = %d, %v", n, err)
	}
}

func TestBulkValidation(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	for _, in := range []BulkInput{
		{IDs: []int64{1}, Action: "explode"},
		{IDs: []int64{1}, Action: "archive"},
		{IDs: []int64{1}, Action: "unarchive"},
		{IDs: []int64{1}, Action: "tag"},
		{IDs: []int64{1}, Action: "untag", Tags: []string{" "}},
		{Action: "read"},
		{Query: &SearchInput{Q: "(x"}, Action: "read"},
	} {
		var ve *ValidationError
		if _, err := e.svc.Bulk(ctx, in); !errors.As(err, &ve) {
			t.Fatalf("Bulk(%+v) err = %v, want ValidationError", in, err)
		}
	}
}
