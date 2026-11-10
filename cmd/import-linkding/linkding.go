package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// requestTimeout bounds every JSON request. Asset downloads and uploads are
// only bounded by the run's context: large archives can take a while.
const requestTimeout = 60 * time.Second

// Linkding API shapes, as serialized by linkding's bookmarks/api/serializers.py.

type lkBookmark struct {
	ID           int64    `json:"id"`
	URL          string   `json:"url"`
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	Notes        string   `json:"notes"`
	TagNames     []string `json:"tag_names"`
	Unread       bool     `json:"unread"`
	DateAdded    string   `json:"date_added"`
	DateModified string   `json:"date_modified"`
}

type lkAsset struct {
	ID          int64  `json:"id"`
	AssetType   string `json:"asset_type"` // "snapshot" or "upload"
	ContentType string `json:"content_type"`
	DisplayName string `json:"display_name"`
	Status      string `json:"status"` // "pending", "complete" or "failure"
	DateCreated string `json:"date_created"`
}

// lkBundle tag fields are space-separated strings (linkding queries.py parses
// them with parse_tag_string(..., " ")).
type lkBundle struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	Search       string `json:"search"`
	AnyTags      string `json:"any_tags"`
	AllTags      string `json:"all_tags"`
	ExcludedTags string `json:"excluded_tags"`
	FilterUnread string `json:"filter_unread"` // "off", "yes" or "no"
	Order        int    `json:"order"`
}

type lkPage[T any] struct {
	Count   int     `json:"count"`
	Next    *string `json:"next"`
	Results []T     `json:"results"`
}

// linkding is a read-only client for the linkding REST API.
type linkding struct {
	base  *url.URL // no trailing slash; may carry a context path
	token string
	c     *http.Client
}

func (l *linkding) resolve(pathAndQuery string) string {
	return l.base.String() + pathAndQuery
}

func (l *linkding) request(ctx context.Context, rawURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Token "+l.token)
	return l.c.Do(req)
}

func (l *linkding) getJSON(ctx context.Context, pathAndQuery string, v any) error {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	u := l.resolve(pathAndQuery)
	res, err := l.request(ctx, u)
	if err != nil {
		return fmt.Errorf("linkding: GET %s: %w", u, err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("linkding: GET %s: %s", u, statusError(res))
	}
	if err := json.NewDecoder(res.Body).Decode(v); err != nil {
		return fmt.Errorf("linkding: GET %s: decode: %w", u, err)
	}
	return nil
}

// walk calls fn for every item of a paginated list, following "next", and
// returns the list's total ("count" of the first page). fn returning false
// stops the walk.
//
// Only the path and query of "next" are used, resolved against the configured
// base URL: behind a proxy linkding may report another scheme or host, and the
// token must never go anywhere but the configured server.
func walk[T any](ctx context.Context, l *linkding, path string, fn func(T) bool) (int, error) {
	seen := map[string]bool{}
	total := -1
	for next := path; next != ""; {
		if seen[next] {
			return total, fmt.Errorf("linkding: pagination loop at %s", next)
		}
		seen[next] = true
		var p lkPage[T]
		if err := l.getJSON(ctx, next, &p); err != nil {
			return total, err
		}
		if total < 0 {
			total = p.Count
		}
		for _, item := range p.Results {
			if !fn(item) {
				return total, nil
			}
		}
		next = ""
		if p.Next != nil && *p.Next != "" {
			nu, err := url.Parse(*p.Next)
			if err != nil {
				return total, fmt.Errorf("linkding: bad next URL %q: %w", *p.Next, err)
			}
			next = strings.TrimPrefix(nu.Path, l.base.Path)
			if nu.RawQuery != "" {
				next += "?" + nu.RawQuery
			}
		}
	}
	return total, nil
}

// download opens an asset's content. The caller closes the body.
func (l *linkding) download(ctx context.Context, bookmarkID, assetID int64) (*http.Response, error) {
	u := l.resolve(fmt.Sprintf("/api/bookmarks/%d/assets/%d/download/", bookmarkID, assetID))
	res, err := l.request(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("download asset %d: %w", assetID, err)
	}
	if res.StatusCode != http.StatusOK {
		defer res.Body.Close()
		return nil, fmt.Errorf("download asset %d: %s", assetID, statusError(res))
	}
	return res, nil
}

// statusError describes a failed response: the status plus the "detail" and
// "errors" of a DRF or Huma problem body, or a short excerpt of the body.
func statusError(res *http.Response) string {
	body, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
	msg := res.Status
	var problem struct {
		Detail string `json:"detail"`
		Errors []struct {
			Message  string `json:"message"`
			Location string `json:"location"`
		} `json:"errors"`
	}
	if json.Unmarshal(body, &problem) == nil && (problem.Detail != "" || len(problem.Errors) > 0) {
		if problem.Detail != "" {
			msg += ": " + problem.Detail
		}
		for _, e := range problem.Errors {
			msg += "; " + strings.TrimPrefix(e.Location, "body.") + " " + e.Message
		}
		return msg
	}
	if s := strings.TrimSpace(string(body)); s != "" {
		if len(s) > 200 {
			s = s[:200] + "..."
		}
		msg += ": " + s
	}
	return msg
}

// parseTime parses a linkding timestamp (DRF DateTimeField with USE_TZ=True,
// e.g. "2020-09-26T09:46:23.006313Z").
func parseTime(s string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, s)
}
