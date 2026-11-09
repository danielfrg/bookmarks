package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/danielfrg/bookmarks/internal/bookmarks"
	"github.com/danielfrg/bookmarks/internal/service"
	"github.com/danielgtaylor/huma/v2"
)

// bookmarkJSON is the bookmark shape shared by the web UI, the extension and
// the import script.
type bookmarkJSON struct {
	ID               int64     `json:"id"`
	URL              string    `json:"url"`
	Title            string    `json:"title"`
	Description      string    `json:"description"`
	Notes            string    `json:"notes"`
	Tags             []string  `json:"tags"`
	Unread           bool      `json:"unread"`
	IsArchived       bool      `json:"is_archived"`
	FaviconURL       string    `json:"favicon_url"`
	PreviewImageURL  string    `json:"preview_image_url"`
	WebArchiveURL    string    `json:"web_archive_url"`
	DateAdded        time.Time `json:"date_added"`
	DateModified     time.Time `json:"date_modified"`
	ArchiveCount     int       `json:"archive_count"`
	BasicCaptureOnly bool      `json:"basic_capture_only"`
	ReaderStatus     string    `json:"reader_status"`
}

type searchHitJSON struct {
	bookmarkJSON
	Snippet string `json:"snippet"`
}

func toBookmark(b bookmarks.Bookmark) bookmarkJSON {
	j := bookmarkJSON{
		ID: b.ID, URL: b.URL, Title: b.Title, Description: b.Description, Notes: b.Notes,
		Tags: b.Tags, Unread: b.Unread, IsArchived: b.Archived, WebArchiveURL: b.WebArchiveURL,
		DateAdded: b.DateAdded.UTC(), DateModified: b.DateModified.UTC(),
		ArchiveCount: b.ArchiveCount, BasicCaptureOnly: b.BasicCaptureOnly, ReaderStatus: string(b.ReaderStatus),
	}
	if j.Tags == nil {
		j.Tags = []string{}
	}
	if j.ReaderStatus == "" {
		j.ReaderStatus = string(bookmarks.ReaderNone)
	}
	if b.FaviconFile != "" {
		j.FaviconURL = "/_/favicons/" + b.FaviconFile
	}
	if b.PreviewFile != "" {
		j.PreviewImageURL = "/_/previews/" + b.PreviewFile
	}
	return j
}

type bookmarkOutput struct {
	Status int
	Body   bookmarkJSON
}

type idInput struct {
	ID int64 `path:"id" minimum:"1"`
}

type listInput struct {
	Q           string `query:"q"`
	Scope       string `query:"scope" doc:"active (default), archived or all"`
	Unread      string `query:"unread" doc:"yes or no"`
	Sort        string `query:"sort"`
	SavedSearch int64  `query:"saved_search" minimum:"0"`
	Limit       int    `query:"limit" minimum:"0"`
	Offset      int    `query:"offset" minimum:"0"`
}

type listOutput struct {
	Body struct {
		Count   int             `json:"count"`
		Results []searchHitJSON `json:"results"`
	}
}

type createBody struct {
	_            struct{}   `additionalProperties:"true"`
	URL          string     `json:"url"`
	Title        *string    `json:"title,omitempty"`
	Description  *string    `json:"description,omitempty"`
	Notes        *string    `json:"notes,omitempty"`
	Tags         *[]string  `json:"tags,omitempty"`
	Unread       *bool      `json:"unread,omitempty"`
	IsArchived   *bool      `json:"is_archived,omitempty"`
	DateAdded    *time.Time `json:"date_added,omitempty"`
	DateModified *time.Time `json:"date_modified,omitempty"`
}

type patchBody struct {
	_            struct{}   `additionalProperties:"true"`
	URL          string     `json:"url,omitempty"`
	Title        *string    `json:"title,omitempty"`
	Description  *string    `json:"description,omitempty"`
	Notes        *string    `json:"notes,omitempty"`
	Tags         *[]string  `json:"tags,omitempty"`
	Unread       *bool      `json:"unread,omitempty"`
	IsArchived   *bool      `json:"is_archived,omitempty"`
	DateAdded    *time.Time `json:"date_added,omitempty"`
	DateModified *time.Time `json:"date_modified,omitempty"`
}

type createInput struct {
	Capture string `query:"capture" default:"server" doc:"server (capture the page in the background) or none"`
	Body    createBody
}

type patchInput struct {
	ID   int64 `path:"id" minimum:"1"`
	Body patchBody
}

type checkInput struct {
	URL string `query:"url"`
}

type checkOutput struct {
	Body struct {
		Bookmark         *bookmarkJSON `json:"bookmark"`
		BasicCaptureOnly bool          `json:"basic_capture_only"`
		Metadata         struct {
			Title           string `json:"title"`
			Description     string `json:"description"`
			PreviewImageURL string `json:"preview_image_url"`
		} `json:"metadata"`
		AutoTags []string `json:"auto_tags"`
	}
}

type readerInput struct {
	ID       int64 `path:"id" minimum:"1"`
	Offset   int   `query:"offset" minimum:"0"`
	MaxChars int   `query:"max_chars" minimum:"0"`
}

type readerOutput struct {
	Body struct {
		Status      string     `json:"status"`
		Error       string     `json:"error"`
		Markdown    string     `json:"markdown"`
		Offset      int        `json:"offset"`
		TotalChars  int        `json:"total_chars"`
		NextOffset  *int       `json:"next_offset" nullable:"true"`
		WordCount   int        `json:"word_count"`
		ExtractedAt *time.Time `json:"extracted_at" nullable:"true"`
	}
}

type emptyOutput struct{}

func registerBookmarks(api huma.API, svc *service.Service) {
	huma.Register(api, huma.Operation{OperationID: "list-bookmarks", Method: http.MethodGet, Path: "/_/api/bookmarks", Summary: "Search bookmarks"},
		func(ctx context.Context, in *listInput) (*listOutput, error) {
			res, err := svc.Search(ctx, service.SearchInput{
				Q: in.Q, Scope: in.Scope, Unread: in.Unread, Sort: in.Sort, SavedSearchID: in.SavedSearch,
				Limit: in.Limit, Offset: in.Offset, Snippets: in.Q != "",
			})
			if err != nil {
				return nil, apiError(err)
			}
			out := &listOutput{}
			out.Body.Count = res.Total
			out.Body.Results = make([]searchHitJSON, 0, len(res.Hits))
			for _, h := range res.Hits {
				out.Body.Results = append(out.Body.Results, searchHitJSON{bookmarkJSON: toBookmark(h.Bookmark), Snippet: h.Snippet})
			}
			return out, nil
		})

	huma.Register(api, huma.Operation{OperationID: "save-bookmark", Method: http.MethodPost, Path: "/_/api/bookmarks", Summary: "Create a bookmark, or update the one with the same URL",
		Description: "Returns 201 when created and 200 when merged into an existing bookmark."},
		func(ctx context.Context, in *createInput) (*bookmarkOutput, error) {
			b, created, err := svc.Save(ctx, service.SaveInput{
				URL: in.Body.URL, Title: in.Body.Title, Description: in.Body.Description, Notes: in.Body.Notes,
				Tags: in.Body.Tags, Unread: in.Body.Unread, Archived: in.Body.IsArchived,
				DateAdded: in.Body.DateAdded, DateModified: in.Body.DateModified,
				Capture: service.CaptureMode(in.Capture),
			})
			if err != nil {
				return nil, apiError(err)
			}
			status := http.StatusOK
			if created {
				status = http.StatusCreated
			}
			return &bookmarkOutput{Status: status, Body: toBookmark(b)}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "check-bookmark", Method: http.MethodGet, Path: "/_/api/bookmarks/check", Summary: "Look up a URL before saving it"},
		func(ctx context.Context, in *checkInput) (*checkOutput, error) {
			res, err := svc.Check(ctx, in.URL)
			if err != nil {
				return nil, apiError(err)
			}
			out := &checkOutput{}
			if res.Bookmark != nil {
				j := toBookmark(*res.Bookmark)
				out.Body.Bookmark = &j
			}
			out.Body.BasicCaptureOnly = res.BasicCaptureOnly
			out.Body.Metadata.Title = res.Metadata.Title
			out.Body.Metadata.Description = res.Metadata.Description
			out.Body.Metadata.PreviewImageURL = res.Metadata.PreviewImageURL
			out.Body.AutoTags = res.AutoTags
			if out.Body.AutoTags == nil {
				out.Body.AutoTags = []string{}
			}
			return out, nil
		})

	huma.Register(api, huma.Operation{OperationID: "get-bookmark", Method: http.MethodGet, Path: "/_/api/bookmarks/{id}", Summary: "Get a bookmark"},
		func(ctx context.Context, in *idInput) (*bookmarkOutput, error) {
			b, err := svc.Get(ctx, in.ID)
			if err != nil {
				return nil, apiError(err)
			}
			return &bookmarkOutput{Status: http.StatusOK, Body: toBookmark(b)}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "update-bookmark", Method: http.MethodPatch, Path: "/_/api/bookmarks/{id}", Summary: "Update the provided fields of a bookmark"},
		func(ctx context.Context, in *patchInput) (*bookmarkOutput, error) {
			b, err := svc.Update(ctx, in.ID, service.SaveInput{
				URL: in.Body.URL, Title: in.Body.Title, Description: in.Body.Description, Notes: in.Body.Notes,
				Tags: in.Body.Tags, Unread: in.Body.Unread, Archived: in.Body.IsArchived,
				DateAdded: in.Body.DateAdded, DateModified: in.Body.DateModified,
			})
			if err != nil {
				return nil, apiError(err)
			}
			return &bookmarkOutput{Status: http.StatusOK, Body: toBookmark(b)}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "delete-bookmark", Method: http.MethodDelete, Path: "/_/api/bookmarks/{id}", Summary: "Delete a bookmark", DefaultStatus: http.StatusNoContent},
		func(ctx context.Context, in *idInput) (*emptyOutput, error) {
			if err := svc.Delete(ctx, in.ID); err != nil {
				return nil, apiError(err)
			}
			return &emptyOutput{}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "capture-bookmark", Method: http.MethodPost, Path: "/_/api/bookmarks/{id}/capture", Summary: "Request a server capture", DefaultStatus: http.StatusAccepted},
		func(ctx context.Context, in *idInput) (*emptyOutput, error) {
			if err := svc.RequestCapture(ctx, in.ID); err != nil {
				return nil, apiError(err)
			}
			return &emptyOutput{}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "extract-reader", Method: http.MethodPost, Path: "/_/api/bookmarks/{id}/reader/extract", Summary: "Request reader text extraction", DefaultStatus: http.StatusAccepted},
		func(ctx context.Context, in *idInput) (*emptyOutput, error) {
			if err := svc.RequestExtract(ctx, in.ID); err != nil {
				return nil, apiError(err)
			}
			return &emptyOutput{}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "get-reader", Method: http.MethodGet, Path: "/_/api/bookmarks/{id}/reader", Summary: "Get a page of reader text (markdown)"},
		func(ctx context.Context, in *readerInput) (*readerOutput, error) {
			p, err := svc.Reader(ctx, in.ID, in.Offset, in.MaxChars)
			if err != nil {
				return nil, apiError(err)
			}
			out := &readerOutput{}
			out.Body.Status = string(p.Status)
			if out.Body.Status == "" {
				out.Body.Status = string(bookmarks.ReaderNone)
			}
			out.Body.Error = p.Error
			out.Body.Markdown = p.Markdown
			out.Body.Offset = p.Offset
			out.Body.TotalChars = p.TotalChars
			out.Body.NextOffset = p.NextOffset
			out.Body.WordCount = p.WordCount
			if !p.ExtractedAt.IsZero() {
				t := p.ExtractedAt.UTC()
				out.Body.ExtractedAt = &t
			}
			return out, nil
		})
}
