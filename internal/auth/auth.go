// Package auth holds the API token helpers shared by the REST API and MCP.
package auth

import (
	"crypto/sha256"
	"crypto/subtle"
	"strings"
)

// TokenFromHeader extracts the token of an "Authorization: Bearer <t>" or
// "Authorization: Token <t>" header; the scheme is case-insensitive. Other
// schemes (e.g. Basic added by a reverse proxy) report ok=false.
func TokenFromHeader(h string) (tok string, ok bool) {
	scheme, rest, _ := strings.Cut(strings.TrimSpace(h), " ")
	if !strings.EqualFold(scheme, "Bearer") && !strings.EqualFold(scheme, "Token") {
		return "", false
	}
	return strings.TrimSpace(rest), true
}

// Equal compares tokens in constant time. An empty token never matches.
func Equal(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	x, y := sha256.Sum256([]byte(a)), sha256.Sum256([]byte(b))
	return subtle.ConstantTimeCompare(x[:], y[:]) == 1
}
