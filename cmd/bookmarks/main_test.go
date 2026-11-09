package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// tinyPNG is a valid 1x1 PNG.
var tinyPNG, _ = base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg==")

const uniqueWord = "zyxwvuniquetoken"

func articleHTML() string {
	var body strings.Builder
	for i := 0; i < 30; i++ {
		body.WriteString("Gardens grow slowly when patient people tend them with care. ")
	}
	return `<!doctype html><html><head><title>Garden Notes</title></head><body><article><h1>Garden Notes</h1><p>` +
		body.String() + ` The ` + uniqueWord + ` is mentioned only here.</p></article></body></html>`
}

type bearer struct{ next http.RoundTripper }

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer t")
	return b.next.RoundTrip(r)
}

func TestEndToEnd(t *testing.T) {
	clearEnv(t)
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/favicon" {
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(tinyPNG)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, articleHTML())
	}))
	defer site.Close()
	t.Setenv("BOOKMARKS_ADDR", "127.0.0.1:0")
	t.Setenv("BOOKMARKS_DATA_DIR", t.TempDir())
	t.Setenv("BOOKMARKS_API_TOKEN", "t")
	t.Setenv("BOOKMARKS_ALLOW_PRIVATE_FETCH", "true")
	t.Setenv("BOOKMARKS_FAVICON_PROVIDER", site.URL+"/favicon?u={url}")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	addrCh := make(chan string, 1)
	done := make(chan error, 1)
	go func() { done <- run(ctx, func(addr string) { addrCh <- addr }) }()
	var base string
	select {
	case addr := <-addrCh:
		base = "http://" + addr
	case err := <-done:
		t.Fatalf("run exited early: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("server did not start")
	}

	client := &http.Client{Transport: bearer{http.DefaultTransport}, Timeout: 10 * time.Second}
	do := func(method, path, contentType string, body io.Reader) (int, []byte) {
		t.Helper()
		req, err := http.NewRequest(method, base+path, body)
		if err != nil {
			t.Fatal(err)
		}
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		data, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, data
	}

	status, data := do("POST", "/api/bookmarks?capture=server", "application/json",
		strings.NewReader(fmt.Sprintf(`{"url":%q}`, site.URL+"/article")))
	if status != 201 && status != 200 {
		t.Fatalf("create: %d %s", status, data)
	}
	var created struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(data, &created); err != nil || created.ID == 0 {
		t.Fatalf("create body: %s", data)
	}

	type bookmark struct {
		Title            string `json:"title"`
		ReaderStatus     string `json:"reader_status"`
		BasicCaptureOnly bool   `json:"basic_capture_only"`
	}
	get := func() bookmark {
		t.Helper()
		_, data := do("GET", fmt.Sprintf("/api/bookmarks/%d", created.ID), "", nil)
		var b bookmark
		if err := json.Unmarshal(data, &b); err != nil {
			t.Fatalf("%s", data)
		}
		return b
	}
	var b bookmark
	deadline := time.Now().Add(10 * time.Second)
	for {
		if b = get(); b.ReaderStatus == "ok" || time.Now().After(deadline) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if b.ReaderStatus != "ok" || !b.BasicCaptureOnly || b.Title != "Garden Notes" {
		t.Fatalf("after capture: %+v", b)
	}

	status, data = do("GET", "/api/bookmarks?q="+uniqueWord, "", nil)
	// uniqueWord appears only in the article body (not title, URL or notes), so
	// a hit proves the reader text was indexed.
	var found struct {
		Results []struct {
			ID int64 `json:"id"`
		} `json:"results"`
	}
	if err := json.Unmarshal(data, &found); err != nil || status != 200 || len(found.Results) != 1 || found.Results[0].ID != created.ID {
		t.Fatalf("search for body-only word: %d %s", status, data)
	}

	c := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "0"}, nil)
	session, err := c.Connect(context.Background(), &sdk.StreamableClientTransport{Endpoint: base + "/mcp", HTTPClient: client}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	res, err := session.CallTool(context.Background(), &sdk.CallToolParams{Name: "read_bookmark", Arguments: map[string]any{"id": created.ID}})
	if err != nil || res.IsError {
		t.Fatalf("read_bookmark: %v %+v", err, res)
	}
	out, _ := json.Marshal(res.StructuredContent)
	if !strings.Contains(string(out), uniqueWord) {
		t.Fatalf("read_bookmark output lacks the body text: %s", out)
	}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if err := mw.WriteField("source", "extension"); err != nil {
		t.Fatal(err)
	}
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", `form-data; name="file"; filename="page.html"`)
	h.Set("Content-Type", "text/html")
	fw, err := mw.CreatePart(h)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(fw, articleHTML())
	mw.Close()
	status, data = do("POST", fmt.Sprintf("/api/bookmarks/%d/archives", created.ID), mw.FormDataContentType(), &buf)
	if status != 201 {
		t.Fatalf("upload: %d %s", status, data)
	}
	if b = get(); b.BasicCaptureOnly {
		t.Fatalf("basic_capture_only still true after extension archive: %+v", b)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("run did not return after cancel")
	}
}
