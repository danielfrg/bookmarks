package reader

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

func load(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestExtractArticle(t *testing.T) {
	res, err := Extract(context.Background(), "text/html; charset=utf-8", bytes.NewReader(load(t, "article.html")), "https://example.com/post")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Markdown, "## Why readable text matters") {
		t.Errorf("missing heading:\n%s", res.Markdown)
	}
	if !strings.Contains(res.Markdown, "[full deep dive article](https://example.com/deep-dive)") {
		t.Errorf("missing link:\n%s", res.Markdown)
	}
	for _, bad := range []string{"About Us Menu", "Subscribe to our newsletter", "data:image", "!["} {
		if strings.Contains(res.Markdown, bad) {
			t.Errorf("markdown should not contain %q", bad)
		}
	}
	if res.Title == "" {
		t.Error("empty title")
	}
	// Article body is about 430 words (heading and link text included).
	const want = 430
	if res.WordCount < want*9/10 || res.WordCount > want*11/10 {
		t.Errorf("WordCount = %d, want within 10%% of %d", res.WordCount, want)
	}
}

func TestExtractPDF(t *testing.T) {
	res, err := Extract(context.Background(), "application/pdf", bytes.NewReader(load(t, "hello.pdf")), "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Markdown, "Hello reader") {
		t.Errorf("markdown = %q", res.Markdown)
	}
}

func TestExtractFailures(t *testing.T) {
	pdf := load(t, "hello.pdf")
	cases := []struct {
		name string
		ct   string
		body []byte
		want error // nil means any non-nil error
		sent bool
	}{
		{"empty", "text/html", nil, ErrNoContent, true},
		{"nav-only", "text/html", load(t, "nav-only.html"), ErrNoContent, true},
		{"png", "image/png", []byte("\x89PNG"), ErrUnsupported, true},
		{"truncated pdf", "application/pdf", pdf[:len(pdf)/2], nil, false},
		{"garbage pdf", "application/pdf", []byte("not a pdf at all"), nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Extract(context.Background(), c.ct, bytes.NewReader(c.body), "https://example.com/")
			if err == nil {
				t.Fatal("want error")
			}
			if c.sent && !errors.Is(err, c.want) {
				t.Errorf("err = %v, want %v", err, c.want)
			}
		})
	}
}

func TestExtractTooLarge(t *testing.T) {
	r := &zeroReader{n: maxInput + 10}
	if _, err := Extract(context.Background(), "application/pdf", r, ""); err == nil {
		t.Fatal("want error")
	}
}

type zeroReader struct{ n int64 }

func (z *zeroReader) Read(p []byte) (int, error) {
	if z.n <= 0 {
		return 0, errors.New("eof-not-reached")
	}
	n := int64(len(p))
	if n > z.n {
		n = z.n
	}
	z.n -= n
	return int(n), nil
}
