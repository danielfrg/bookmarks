// Package auth holds the API token helpers shared by the REST API and MCP.
package auth

import (
	"context"
	"strings"
)

// Verifier checks a presented API token. Unknown tokens are (false, nil); err
// is set only when verification itself failed (e.g. the database is down).
// *service.Service implements it.
type Verifier interface {
	VerifyToken(ctx context.Context, token string) (bool, error)
}

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
