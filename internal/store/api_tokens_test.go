package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/danielfrg/bookmarks/internal/bookmarks"
)

func TestAPITokensRoundTrip(t *testing.T) {
	s, ctx := openTest(t), context.Background()
	t0 := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	a := &bookmarks.APIToken{Name: "ext", TokenHash: "aaa", Prefix: "abcdefgh", CreatedAt: t0}
	b := &bookmarks.APIToken{Name: "cli", TokenHash: "bbb", Prefix: "ijklmnop", CreatedAt: t0.Add(time.Second)}
	for _, tok := range []*bookmarks.APIToken{a, b} {
		if err := s.CreateAPIToken(ctx, tok); err != nil || tok.ID == 0 {
			t.Fatalf("create: %v %+v", err, tok)
		}
	}
	list, err := s.ListAPITokens(ctx)
	if err != nil || len(list) != 2 || list[0].ID != b.ID || list[1].ID != a.ID {
		t.Fatalf("list (newest first) = %+v %v", list, err)
	}
	if list[1].Name != "ext" || list[1].Prefix != "abcdefgh" || !list[1].CreatedAt.Equal(t0) || list[1].LastUsedAt != nil {
		t.Fatalf("got %+v", list[1])
	}

	got, err := s.FindAPITokenByHash(ctx, "aaa")
	if err != nil || got.ID != a.ID || got.TokenHash != "aaa" {
		t.Fatalf("find: %+v %v", got, err)
	}
	if _, err := s.FindAPITokenByHash(ctx, "zzz"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("find unknown: %v", err)
	}

	used := t0.Add(time.Hour)
	if err := s.TouchAPIToken(ctx, a.ID, used); err != nil {
		t.Fatal(err)
	}
	got, _ = s.FindAPITokenByHash(ctx, "aaa")
	if got.LastUsedAt == nil || !got.LastUsedAt.Equal(used) {
		t.Fatalf("last used = %v", got.LastUsedAt)
	}
	if err := s.TouchAPIToken(ctx, 999, used); !errors.Is(err, ErrNotFound) {
		t.Fatalf("touch unknown: %v", err)
	}

	if err := s.DeleteAPIToken(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteAPIToken(ctx, a.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete twice: %v", err)
	}
	if _, err := s.FindAPITokenByHash(ctx, "aaa"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted token found: %v", err)
	}
}

func TestAPITokenHashUnique(t *testing.T) {
	s, ctx := openTest(t), context.Background()
	if err := s.CreateAPIToken(ctx, &bookmarks.APIToken{Name: "a", TokenHash: "same", Prefix: "x", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateAPIToken(ctx, &bookmarks.APIToken{Name: "b", TokenHash: "same", Prefix: "y", CreatedAt: time.Now()}); err == nil {
		t.Fatal("duplicate hash accepted")
	}
}
