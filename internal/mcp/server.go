// Package mcp serves the Bookmarks MCP endpoint: search, read, list tags, add.
package mcp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/danielfrg/bookmarks/internal/bookmarks"
	"github.com/danielfrg/bookmarks/internal/service"
	"github.com/danielfrg/bookmarks/internal/store"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	defaultLimit    = 10
	maxLimit        = 50
	defaultMaxChars = 8000
	maxMaxChars     = 30000
)

type SearchInput struct {
	Query  string `json:"query" jsonschema:"search text; supports #tag, !unread, !untagged, quoted phrases and and/or/not"`
	Scope  string `json:"scope,omitempty" jsonschema:"all (default), active or archived"`
	Limit  int    `json:"limit,omitempty" jsonschema:"results per page, default 10, max 50"`
	Offset int    `json:"offset,omitempty" jsonschema:"results to skip; pass next_offset from the previous page"`
}

type SearchResult struct {
	ID           int64    `json:"id"`
	Title        string   `json:"title"`
	URL          string   `json:"url"`
	Tags         []string `json:"tags"`
	DateAdded    string   `json:"date_added"`
	Snippet      string   `json:"snippet"`
	ReaderStatus string   `json:"reader_status" jsonschema:"ok, failed or none; read_bookmark has text only when ok"`
}

type SearchOutput struct {
	Results    []SearchResult `json:"results"`
	Total      int            `json:"total"`
	NextOffset *int           `json:"next_offset"`
}

type ReadInput struct {
	ID       int64 `json:"id" jsonschema:"bookmark id from search_bookmarks"`
	Offset   int   `json:"offset,omitempty" jsonschema:"character offset into the text; pass next_offset to continue"`
	MaxChars int   `json:"max_chars,omitempty" jsonschema:"page size in characters, default 8000, max 30000"`
}

type ReaderText struct {
	Markdown   string `json:"markdown"`
	Offset     int    `json:"offset"`
	TotalChars int    `json:"total_chars"`
	NextOffset *int   `json:"next_offset"`
	WordCount  int    `json:"word_count"`
}

type ReadOutput struct {
	ID           int64       `json:"id"`
	URL          string      `json:"url"`
	Title        string      `json:"title"`
	Description  string      `json:"description"`
	Tags         []string    `json:"tags"`
	Notes        string      `json:"notes"`
	DateAdded    string      `json:"date_added"`
	ReaderText   *ReaderText `json:"reader_text"`
	ReaderStatus string      `json:"reader_status"`
	ReaderError  string      `json:"reader_error,omitempty"`
}

type Tag struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}
type ListTagsInput struct{}
type ListTagsOutput struct {
	Tags []Tag `json:"tags"`
}

type AddInput struct {
	URL    string   `json:"url" jsonschema:"http(s) URL to save"`
	Title  *string  `json:"title,omitempty" jsonschema:"optional; fetched from the page when omitted"`
	Tags   []string `json:"tags,omitempty"`
	Notes  *string  `json:"notes,omitempty"`
	Unread *bool    `json:"unread,omitempty" jsonschema:"mark as to-read"`
}
type AddOutput struct {
	ID      int64  `json:"id"`
	Created bool   `json:"created" jsonschema:"false when the URL was already saved and that bookmark was updated"`
	URL     string `json:"url"`
}

func annotation(read bool) *sdk.ToolAnnotations {
	no := false
	return &sdk.ToolAnnotations{ReadOnlyHint: read, IdempotentHint: true, DestructiveHint: &no, OpenWorldHint: &no}
}

// safeError maps errors to messages that never leak internals.
func safeError(err error) error {
	if err == nil {
		return nil
	}
	var validation *service.ValidationError
	switch {
	case errors.Is(err, store.ErrNotFound):
		return errors.New("Bookmark not found")
	case errors.As(err, &validation):
		return errors.New(validation.Error())
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return errors.New("Request canceled")
	default:
		return errors.New("Bookmarks unavailable; try again")
	}
}

func ts(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func tagsOrEmpty(t []string) []string {
	if t == nil {
		return []string{}
	}
	return t
}

func NewHandler(svc *service.Service, allowedHosts []string) http.Handler {
	server := sdk.NewServer(&sdk.Implementation{Name: "bookmarks", Title: "Bookmarks", Version: "0.1.0"}, &sdk.ServerOptions{Instructions: serverInstructions})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "search_bookmarks",
		Description: "Search saved bookmarks by topic across titles, tags, notes and the full saved page text. Returns matches with a snippet, ordered by relevance. Keyword-based: retry with other phrasings when results are thin.",
		Annotations: annotation(true),
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in SearchInput) (*sdk.CallToolResult, SearchOutput, error) {
		out := SearchOutput{Results: []SearchResult{}}
		limit := in.Limit
		switch {
		case limit == 0:
			limit = defaultLimit
		case limit < 0 || limit > maxLimit:
			return nil, out, fmt.Errorf("limit: must be between 1 and %d", maxLimit)
		}
		if in.Offset < 0 {
			return nil, out, errors.New("offset: must not be negative")
		}
		scope := in.Scope
		if scope == "" {
			scope = "all"
		}
		res, err := svc.Search(ctx, service.SearchInput{Q: in.Query, Scope: scope, Sort: "relevance", Limit: limit, Offset: in.Offset, Snippets: true})
		if err != nil {
			return nil, out, safeError(err)
		}
		for _, h := range res.Hits {
			out.Results = append(out.Results, SearchResult{
				ID: h.ID, Title: h.Title, URL: h.URL, Tags: tagsOrEmpty(h.Tags), DateAdded: ts(h.DateAdded),
				Snippet: h.Snippet, ReaderStatus: string(h.ReaderStatus),
			})
		}
		out.Total = res.Total
		if next := in.Offset + len(res.Hits); len(res.Hits) > 0 && next < res.Total {
			out.NextOffset = &next
		}
		return nil, out, nil
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "read_bookmark",
		Description: "Read a bookmark's saved page text as markdown, in pages. Continue with offset=next_offset until next_offset is null. reader_text is null when no text was extracted (see reader_status).",
		Annotations: annotation(true),
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in ReadInput) (*sdk.CallToolResult, ReadOutput, error) {
		maxChars := in.MaxChars
		switch {
		case maxChars == 0:
			maxChars = defaultMaxChars
		case maxChars < 0 || maxChars > maxMaxChars:
			return nil, ReadOutput{}, fmt.Errorf("max_chars: must be between 1 and %d", maxMaxChars)
		}
		p, err := svc.Reader(ctx, in.ID, in.Offset, maxChars)
		if err != nil {
			return nil, ReadOutput{}, safeError(err)
		}
		b := p.Bookmark
		out := ReadOutput{
			ID: b.ID, URL: b.URL, Title: b.Title, Description: b.Description, Tags: tagsOrEmpty(b.Tags),
			Notes: b.Notes, DateAdded: ts(b.DateAdded), ReaderStatus: string(p.Status),
		}
		switch p.Status {
		case bookmarks.ReaderOK:
			out.ReaderText = &ReaderText{Markdown: p.Markdown, Offset: p.Offset, TotalChars: p.TotalChars, NextOffset: p.NextOffset, WordCount: p.WordCount}
		case bookmarks.ReaderFailed:
			out.ReaderError = p.Error
		}
		return nil, out, nil
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "list_tags",
		Description: "List all tags with the number of bookmarks using each.",
		Annotations: annotation(true),
	}, func(ctx context.Context, _ *sdk.CallToolRequest, _ ListTagsInput) (*sdk.CallToolResult, ListTagsOutput, error) {
		tags, err := svc.Tags(ctx)
		out := ListTagsOutput{Tags: []Tag{}}
		if err != nil {
			return nil, out, safeError(err)
		}
		for _, t := range tags {
			out.Tags = append(out.Tags, Tag{Name: t.Name, Count: t.Count})
		}
		return nil, out, nil
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "add_bookmark",
		Description: "Save a link. Only use when asked to save a link. Saving a URL that already exists keeps the existing bookmark: its tags, notes and title are preserved and any new tags are added.",
		Annotations: annotation(false),
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in AddInput) (*sdk.CallToolResult, AddOutput, error) {
		si := service.SaveInput{URL: in.URL, Capture: service.CaptureServer}
		existing, err := svc.FindByURL(ctx, in.URL)
		switch {
		case err == nil:
			// Never overwrite what the user curated: only add new tags.
			tags := append(append([]string{}, existing.Tags...), in.Tags...)
			si.Tags = &tags
		case errors.Is(err, store.ErrNotFound):
			si.Title, si.Notes, si.Unread = in.Title, in.Notes, in.Unread
			if in.Tags != nil {
				si.Tags = &in.Tags
			}
		default:
			return nil, AddOutput{}, safeError(err)
		}
		b, created, err := svc.Save(ctx, si)
		if err != nil {
			return nil, AddOutput{}, safeError(err)
		}
		return nil, AddOutput{ID: b.ID, Created: created, URL: b.URL}, nil
	})

	return transport(server, allowedHosts, svc)
}
