package mcp

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/danielfrg/bookmarks/internal/bookmarks"
	"github.com/danielfrg/bookmarks/internal/capture"
	"github.com/danielfrg/bookmarks/internal/fetch"
	"github.com/danielfrg/bookmarks/internal/jobs"
	"github.com/danielfrg/bookmarks/internal/service"
	"github.com/danielfrg/bookmarks/internal/store"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type noopQueue struct{}

func (noopQueue) Enqueue(context.Context, jobs.Kind, int64) error { return nil }

type fakeCapturer struct{}

func (fakeCapturer) Capture(context.Context, string) (capture.Result, error) {
	return capture.Result{}, nil
}

type env struct {
	svc *service.Service
	st  *store.SQLite
	srv *httptest.Server
}

func newEnv(t *testing.T) *env {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "bookmarks.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	svc := service.New(st, fetch.NewClient(fetch.Options{AllowPrivate: true}), fakeCapturer{}, noopQueue{}, service.Options{
		DataDir: dir, Logger: log.New(io.Discard, "", 0),
	})
	srv := httptest.NewServer(NewHandler(svc, nil))
	t.Cleanup(srv.Close)
	return &env{svc: svc, st: st, srv: srv}
}

func (e *env) session(t *testing.T) *sdk.ClientSession {
	t.Helper()
	c := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "0"}, nil)
	s, err := c.Connect(context.Background(), &sdk.StreamableClientTransport{Endpoint: e.srv.URL}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// call runs a tool and returns the structured content (nil when it is a tool error).
func call(t *testing.T, s *sdk.ClientSession, name string, args map[string]any) (map[string]any, *sdk.CallToolResult) {
	t.Helper()
	res, err := s.CallTool(context.Background(), &sdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		return nil, res
	}
	m, ok := res.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("structured content: %#v", res.StructuredContent)
	}
	return m, res
}

func errText(res *sdk.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*sdk.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

func (e *env) seed(t *testing.T, url, title string, tags []string, md string) int64 {
	t.Helper()
	ctx := context.Background()
	b, _, err := e.svc.Save(ctx, service.SaveInput{URL: url, Title: &title, Tags: &tags, Capture: service.CaptureNone})
	if err != nil {
		t.Fatal(err)
	}
	if md != "" {
		err := e.st.PutReaderText(ctx, bookmarks.ReaderText{BookmarkID: b.ID, Status: bookmarks.ReaderOK, Markdown: md, WordCount: len(strings.Fields(md))})
		if err != nil {
			t.Fatal(err)
		}
	}
	return b.ID
}

func post(t *testing.T, url, host, auth string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest("POST", url, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if host != "" {
		req.Host = host
	}
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}
