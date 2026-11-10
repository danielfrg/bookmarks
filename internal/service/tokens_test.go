package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"log"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/danielfrg/bookmarks/internal/fetch"
	"github.com/danielfrg/bookmarks/internal/store"
)

func TestCreateTokenReturnsPlaintextOnce(t *testing.T) {
	e, ctx := newEnv(t), context.Background()
	info, plain, err := e.svc.CreateToken(ctx, "  Chrome extension ")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(plain, "bk_") || len(plain) != 3+43 {
		t.Fatalf("plaintext %q", plain)
	}
	if info.ID == 0 || info.Name != "Chrome extension" || info.Prefix != plain[3:11] || !info.CreatedAt.Equal(fixedNow) || info.LastUsedAt != nil {
		t.Fatalf("info %+v", info)
	}
	stored, err := e.st.FindAPITokenByHash(ctx, sha256Hex(plain))
	if err != nil || stored.ID != info.ID || stored.TokenHash == plain || strings.Contains(stored.TokenHash, plain[3:]) {
		t.Fatalf("stored %+v %v", stored, err)
	}

	_, other, err := e.svc.CreateToken(ctx, "other")
	if err != nil || other == plain {
		t.Fatalf("second token %q %v", other, err)
	}
	list, err := e.svc.ListTokens(ctx)
	if err != nil || len(list) != 2 {
		t.Fatalf("list %+v %v", list, err)
	}
}

func TestCreateTokenValidatesName(t *testing.T) {
	e, ctx := newEnv(t), context.Background()
	for _, name := range []string{"", "   ", strings.Repeat("x", 101)} {
		var ve *ValidationError
		if _, _, err := e.svc.CreateToken(ctx, name); !errors.As(err, &ve) || ve.Field != "name" {
			t.Errorf("%q: err = %v", name, err)
		}
	}
	if _, _, err := e.svc.CreateToken(ctx, strings.Repeat("é", 100)); err != nil {
		t.Fatalf("100 characters rejected: %v", err)
	}
}

func TestVerifyTokenAndThrottledLastUsed(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "bookmarks.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	now := fixedNow
	svc := New(st, fetch.NewClient(fetch.Options{}), &fakeCapturer{}, &fakeQueue{}, Options{
		DataDir: dir, Now: func() time.Time { return now }, Logger: log.New(io.Discard, "", 0),
	})
	ctx := context.Background()
	info, plain, err := svc.CreateToken(ctx, "t")
	if err != nil {
		t.Fatal(err)
	}
	lastUsed := func() *time.Time {
		t.Helper()
		got, err := st.FindAPITokenByHash(ctx, sha256Hex(plain))
		if err != nil {
			t.Fatal(err)
		}
		return got.LastUsedAt
	}

	for _, bad := range []string{"", "bk_nope", plain + "x", strings.ToUpper(plain)} {
		if ok, err := svc.VerifyToken(ctx, bad); ok || err != nil {
			t.Errorf("%q: ok=%v err=%v", bad, ok, err)
		}
	}
	if lastUsed() != nil {
		t.Fatal("failed verifications touched the token")
	}

	if ok, err := svc.VerifyToken(ctx, plain); !ok || err != nil {
		t.Fatalf("valid token: ok=%v err=%v", ok, err)
	}
	if lu := lastUsed(); lu == nil || !lu.Equal(fixedNow) {
		t.Fatalf("last used = %v", lu)
	}
	now = fixedNow.Add(59 * time.Second)
	if ok, _ := svc.VerifyToken(ctx, plain); !ok {
		t.Fatal("valid token rejected")
	}
	if lu := lastUsed(); !lu.Equal(fixedNow) {
		t.Fatalf("touched within a minute: %v", lu)
	}
	now = fixedNow.Add(time.Minute)
	if ok, _ := svc.VerifyToken(ctx, plain); !ok {
		t.Fatal("valid token rejected")
	}
	if lu := lastUsed(); !lu.Equal(now) {
		t.Fatalf("not touched after a minute: %v", lu)
	}

	if err := svc.DeleteToken(ctx, info.ID); err != nil {
		t.Fatal(err)
	}
	if ok, err := svc.VerifyToken(ctx, plain); ok || err != nil {
		t.Fatalf("deleted token: ok=%v err=%v", ok, err)
	}
	if err := svc.DeleteToken(ctx, info.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("delete twice: %v", err)
	}
}

func TestVerifyTokenReportsStoreErrors(t *testing.T) {
	e, ctx := newEnv(t), context.Background()
	_, plain, err := e.svc.CreateToken(ctx, "t")
	if err != nil {
		t.Fatal(err)
	}
	e.st.Close()
	if ok, err := e.svc.VerifyToken(ctx, plain); ok || err == nil {
		t.Fatalf("closed store: ok=%v err=%v", ok, err)
	}
}

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
