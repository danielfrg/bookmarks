package store

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/danielfrg/bookmarks/internal/bookmarks"
)

func TestBundlesCRUDAndReorder(t *testing.T) {
	s, ctx := openTest(t), context.Background()
	a := &bookmarks.Bundle{Name: "A", Search: "q", AnyTags: []string{"x", "y"}, AllTags: []string{"z"}, ExcludedTags: []string{"w"}, FilterUnread: "yes"}
	b := &bookmarks.Bundle{Name: "B", FilterUnread: "off"}
	if err := s.SaveBundle(ctx, a); err != nil || a.ID == 0 || a.Order != 1 {
		t.Fatalf("a: %v %+v", err, a)
	}
	if err := s.SaveBundle(ctx, b); err != nil || b.Order != 2 {
		t.Fatalf("b: %v %+v", err, b)
	}
	got, err := s.GetBundle(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.AnyTags, []string{"x", "y"}) || !reflect.DeepEqual(got.AllTags, []string{"z"}) ||
		!reflect.DeepEqual(got.ExcludedTags, []string{"w"}) || got.Search != "q" || got.FilterUnread != "yes" || got.DateCreated.IsZero() {
		t.Fatalf("got %+v", got)
	}
	got.Name = "A2"
	got.AnyTags = nil
	if err := s.SaveBundle(ctx, &got); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetBundle(ctx, a.ID)
	if got.Name != "A2" || len(got.AnyTags) != 0 {
		t.Fatalf("got %+v", got)
	}
	if err := s.ReorderBundles(ctx, []int64{b.ID, a.ID}); err != nil {
		t.Fatal(err)
	}
	list, _ := s.ListBundles(ctx)
	if len(list) != 2 || list[0].ID != b.ID || list[1].ID != a.ID {
		t.Fatalf("list = %+v", list)
	}
	if err := s.DeleteBundle(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetBundle(ctx, a.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
	if err := s.DeleteBundle(ctx, a.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestSettingsDefaultsAndRoundTrip(t *testing.T) {
	s, ctx := openTest(t), context.Background()
	got, err := s.GetSettings(ctx)
	if err != nil || !reflect.DeepEqual(got, bookmarks.DefaultSettings()) {
		t.Fatalf("got %+v err %v", got, err)
	}
	want := bookmarks.Settings{WaybackEnabled: true, DefaultUnread: true, AutoTagRules: "a.com go", UI: json.RawMessage(`{"theme":"dark"}`)}
	if err := s.PutSettings(ctx, want); err != nil {
		t.Fatal(err)
	}
	got, err = s.GetSettings(ctx)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v err %v", got, err)
	}
}
