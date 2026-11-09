package store

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestRenameTagMerges(t *testing.T) {
	s, ctx := openTest(t), context.Background()
	a, b := sample("https://a.com", "js"), sample("https://b.com", "javascript")
	s.CreateBookmark(ctx, a)
	s.CreateBookmark(ctx, b)
	tags, _ := s.ListTags(ctx)
	var jsID int64
	for _, tg := range tags {
		if tg.Name == "js" {
			jsID = tg.ID
		}
	}
	if err := s.RenameTag(ctx, jsID, "javascript"); err != nil {
		t.Fatal(err)
	}
	tags, _ = s.ListTags(ctx)
	if len(tags) != 1 || tags[0].Name != "javascript" || tags[0].Count != 2 {
		t.Fatalf("tags = %+v", tags)
	}
	got, _ := s.GetBookmark(ctx, a.ID)
	if !reflect.DeepEqual(got.Tags, []string{"javascript"}) {
		t.Fatalf("tags = %v", got.Tags)
	}
}

func TestRenameAndDeleteTag(t *testing.T) {
	s, ctx := openTest(t), context.Background()
	b := sample("https://a.com", "old")
	s.CreateBookmark(ctx, b)
	tags, _ := s.ListTags(ctx)
	if err := s.RenameTag(ctx, tags[0].ID, "New"); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetBookmark(ctx, b.ID)
	if !reflect.DeepEqual(got.Tags, []string{"new"}) {
		t.Fatalf("tags = %v", got.Tags)
	}
	if err := s.RenameTag(ctx, 999, "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
	if err := s.DeleteTag(ctx, tags[0].ID); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetBookmark(ctx, b.ID)
	if len(got.Tags) != 0 {
		t.Fatalf("tags = %v", got.Tags)
	}
	if err := s.DeleteTag(ctx, tags[0].ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}
