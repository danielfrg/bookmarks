package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/danielfrg/bookmarks/internal/bookmarks"
	"github.com/danielfrg/bookmarks/internal/store"
)

const (
	tokenMarker       = "bk_"
	tokenPrefixLen    = 8
	maxTokenNameRunes = 100
	// lastUsedInterval throttles last-used writes: a token used constantly is
	// written at most once per interval.
	lastUsedInterval = time.Minute
)

// TokenInfo describes an API token without its secret.
type TokenInfo struct {
	ID         int64
	Name       string
	Prefix     string
	CreatedAt  time.Time
	LastUsedAt *time.Time
}

func tokenInfo(t bookmarks.APIToken) TokenInfo {
	return TokenInfo{ID: t.ID, Name: t.Name, Prefix: t.Prefix, CreatedAt: t.CreatedAt, LastUsedAt: t.LastUsedAt}
}

func hashToken(plain string) string {
	h := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(h[:])
}

// CreateToken mints a new API token named name. The plaintext is returned
// only here; the store keeps its SHA-256.
func (s *Service) CreateToken(ctx context.Context, name string) (TokenInfo, string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return TokenInfo{}, "", invalid("name", "is required")
	}
	if utf8.RuneCountInString(name) > maxTokenNameRunes {
		return TokenInfo{}, "", invalid("name", "must be at most 100 characters")
	}
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return TokenInfo{}, "", err
	}
	secret := base64.RawURLEncoding.EncodeToString(raw[:])
	plain := tokenMarker + secret
	t := bookmarks.APIToken{Name: name, TokenHash: hashToken(plain), Prefix: secret[:tokenPrefixLen], CreatedAt: s.now()}
	if err := s.st.CreateAPIToken(ctx, &t); err != nil {
		return TokenInfo{}, "", err
	}
	return tokenInfo(t), plain, nil
}

// ListTokens lists the API tokens, newest first.
func (s *Service) ListTokens(ctx context.Context) ([]TokenInfo, error) {
	list, err := s.st.ListAPITokens(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]TokenInfo, 0, len(list))
	for _, t := range list {
		out = append(out, tokenInfo(t))
	}
	return out, nil
}

// DeleteToken revokes API token id.
func (s *Service) DeleteToken(ctx context.Context, id int64) error {
	return s.st.DeleteAPIToken(ctx, id)
}

// VerifyToken reports whether plain is a current API token. Unknown tokens
// are (false, nil); err is set only when the lookup itself failed. The token's
// last-used time is updated at most once per minute.
func (s *Service) VerifyToken(ctx context.Context, plain string) (bool, error) {
	if plain == "" {
		return false, nil
	}
	t, err := s.st.FindAPITokenByHash(ctx, hashToken(plain))
	if errors.Is(err, store.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	now := s.now()
	if t.LastUsedAt == nil || now.Sub(*t.LastUsedAt) >= lastUsedInterval {
		// Bookkeeping only: a failed write must not reject a valid token.
		// ErrNotFound means the token was deleted since the lookup; nothing to log.
		if err := s.st.TouchAPIToken(ctx, t.ID, now); err != nil && !errors.Is(err, store.ErrNotFound) {
			s.log.Printf("service: update last use of API token %d: %v", t.ID, err)
		}
	}
	return true, nil
}
