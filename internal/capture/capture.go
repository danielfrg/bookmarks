// Package capture saves a snapshot of a web page. The basic capturer is pure
// Go (no JavaScript) and is the server-side fallback for links that were not
// captured by the browser extension.
package capture

import (
	"context"
	"errors"
	"fmt"
)

// Result is a captured page: PDF bytes as-is or self-contained UTF-8 HTML.
type Result struct {
	ContentType string
	Body        []byte
}

// Capturer captures the page at rawURL.
type Capturer interface {
	Capture(ctx context.Context, rawURL string) (Result, error)
}

var (
	// ErrTooLarge means the page exceeded the size limit.
	ErrTooLarge = errors.New("capture: page too large")
	// ErrUnsupported means the page has a content type that cannot be captured.
	ErrUnsupported = errors.New("capture: unsupported content type")
)

// HTTPError is returned when the page responds with a non-2xx status.
type HTTPError struct{ Status int }

func (e *HTTPError) Error() string { return fmt.Sprintf("capture: HTTP %d", e.Status) }
