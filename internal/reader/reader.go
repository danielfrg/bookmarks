// Package reader extracts readable markdown text from saved archives
// (HTML pages and PDFs) so it can be indexed and read by agents.
package reader

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"regexp"
	"strings"
	"unicode"
)

// Result is the extracted reader text.
type Result struct {
	Title     string
	Markdown  string
	WordCount int
}

var (
	ErrNoContent   = errors.New("no readable content")
	ErrUnsupported = errors.New("unsupported content type")
)

const (
	maxInput     = 50 << 20 // cap on archive bytes read
	minWordCount = 20       // fewer words than this is not readable content
)

// Extract converts an archive body to markdown. HTML goes through readability
// and html-to-markdown; PDFs through plain text extraction. pageURL resolves
// relative links in HTML and may be empty.
func Extract(ctx context.Context, contentType string, body io.Reader, pageURL string) (Result, error) {
	mt, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return Result{}, ErrUnsupported
	}
	var res Result
	switch mt {
	case "text/html", "application/xhtml+xml":
		data, err := readCapped(body)
		if err != nil {
			return Result{}, err
		}
		res, err = extractHTML(ctx, data, pageURL)
		if err != nil {
			return Result{}, err
		}
	case "application/pdf":
		data, err := readCapped(body)
		if err != nil {
			return Result{}, err
		}
		res, err = extractPDF(ctx, data)
		if err != nil {
			return Result{}, err
		}
	default:
		return Result{}, ErrUnsupported
	}
	res.WordCount = countWords(res.Markdown)
	if res.WordCount < minWordCount {
		return Result{}, ErrNoContent
	}
	return res, nil
}

func readCapped(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxInput+1))
	if err != nil {
		return nil, fmt.Errorf("read archive: %w", err)
	}
	if len(data) > maxInput {
		return nil, errors.New("archive exceeds 50 MiB limit")
	}
	return data, nil
}

var linkTarget = regexp.MustCompile(`\]\([^)]*\)`)

// countWords counts whitespace-separated words in the markdown after dropping
// link targets, ignoring tokens that are pure markdown punctuation (no letter
// or digit), such as "#", "-", ">" or "|".
func countWords(md string) int {
	md = linkTarget.ReplaceAllString(md, "]")
	n := 0
	for _, f := range strings.Fields(md) {
		if strings.IndexFunc(f, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) >= 0 {
			n++
		}
	}
	return n
}
