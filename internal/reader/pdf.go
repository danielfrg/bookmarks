package reader

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/ledongthuc/pdf"
)

func extractPDF(ctx context.Context, data []byte) (res Result, err error) {
	// The PDF library is known to panic on malformed files.
	defer func() {
		if r := recover(); r != nil {
			res, err = Result{}, fmt.Errorf("read pdf: malformed file (%v)", r)
		}
	}()
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return Result{}, fmt.Errorf("read pdf: %w", err)
	}
	tr, err := r.GetPlainText()
	if err != nil {
		return Result{}, fmt.Errorf("read pdf text: %w", err)
	}
	raw, err := io.ReadAll(tr)
	if err != nil {
		return Result{}, fmt.Errorf("read pdf text: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	md := paragraphs(string(raw))
	if md == "" {
		return Result{}, ErrNoContent
	}
	return Result{Markdown: md}, nil
}

// paragraphs joins consecutive non-blank lines into one paragraph and
// separates paragraphs with a blank line.
func paragraphs(s string) string {
	var out, cur []string
	flush := func() {
		if len(cur) > 0 {
			out = append(out, strings.Join(cur, " "))
			cur = nil
		}
	}
	for _, line := range strings.Split(strings.ReplaceAll(s, "\r", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			flush()
			continue
		}
		cur = append(cur, line)
	}
	flush()
	return strings.Join(out, "\n\n")
}
