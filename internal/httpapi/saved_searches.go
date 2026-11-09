package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/danielfrg/bookmarks/internal/bookmarks"
	"github.com/danielfrg/bookmarks/internal/service"
	"github.com/danielgtaylor/huma/v2"
)

type savedSearchJSON struct {
	ID           int64     `json:"id"`
	Name         string    `json:"name"`
	Query        string    `json:"query"`
	AnyTags      []string  `json:"any_tags"`
	AllTags      []string  `json:"all_tags"`
	ExcludedTags []string  `json:"excluded_tags"`
	FilterUnread string    `json:"filter_unread"`
	Order        int       `json:"order"`
	DateCreated  time.Time `json:"date_created"`
	DateModified time.Time `json:"date_modified"`
}

func toSavedSearch(b bookmarks.SavedSearch) savedSearchJSON {
	nonNil := func(s []string) []string {
		if s == nil {
			return []string{}
		}
		return s
	}
	return savedSearchJSON{
		ID: b.ID, Name: b.Name, Query: b.Query,
		AnyTags: nonNil(b.AnyTags), AllTags: nonNil(b.AllTags), ExcludedTags: nonNil(b.ExcludedTags),
		FilterUnread: b.FilterUnread, Order: b.Order,
		DateCreated: b.DateCreated.UTC(), DateModified: b.DateModified.UTC(),
	}
}

// savedSearchBody is the editable part of a saved search. Everything but the name is
// optional; filter_unread is validated by the service.
type savedSearchBody struct {
	_            struct{} `additionalProperties:"true"`
	Name         string   `json:"name"`
	Query        string   `json:"query,omitempty"`
	AnyTags      []string `json:"any_tags,omitempty"`
	AllTags      []string `json:"all_tags,omitempty"`
	ExcludedTags []string `json:"excluded_tags,omitempty"`
	FilterUnread string   `json:"filter_unread,omitempty"`
}

func (b savedSearchBody) savedSearch(id int64) bookmarks.SavedSearch {
	return bookmarks.SavedSearch{
		ID: id, Name: b.Name, Query: b.Query, AnyTags: b.AnyTags, AllTags: b.AllTags,
		ExcludedTags: b.ExcludedTags, FilterUnread: b.FilterUnread,
	}
}

type savedSearchOutput struct{ Body savedSearchJSON }

type savedSearchListOutput struct{ Body []savedSearchJSON }

type createSavedSearchInput struct{ Body savedSearchBody }

type updateSavedSearchInput struct {
	ID   int64 `path:"id" minimum:"1"`
	Body savedSearchBody
}

type reorderSavedSearchesInput struct {
	Body struct {
		_   struct{} `additionalProperties:"true"`
		IDs []int64  `json:"ids"`
	}
}

func registerSavedSearches(api huma.API, svc *service.Service) {
	huma.Register(api, huma.Operation{OperationID: "list-saved-searches", Method: http.MethodGet, Path: "/api/saved-searches", Summary: "List saved searches in display order"},
		func(ctx context.Context, _ *struct{}) (*savedSearchListOutput, error) {
			list, err := svc.SavedSearches(ctx)
			if err != nil {
				return nil, apiError(err)
			}
			out := &savedSearchListOutput{Body: make([]savedSearchJSON, 0, len(list))}
			for _, b := range list {
				out.Body = append(out.Body, toSavedSearch(b))
			}
			return out, nil
		})

	huma.Register(api, huma.Operation{OperationID: "create-saved-search", Method: http.MethodPost, Path: "/api/saved-searches", Summary: "Create a saved search", DefaultStatus: http.StatusCreated},
		func(ctx context.Context, in *createSavedSearchInput) (*savedSearchOutput, error) {
			b, err := svc.SaveSavedSearch(ctx, in.Body.savedSearch(0))
			if err != nil {
				return nil, apiError(err)
			}
			return &savedSearchOutput{Body: toSavedSearch(b)}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "reorder-saved-searches", Method: http.MethodPost, Path: "/api/saved-searches/reorder", Summary: "Set the saved search order", DefaultStatus: http.StatusNoContent},
		func(ctx context.Context, in *reorderSavedSearchesInput) (*emptyOutput, error) {
			if err := svc.ReorderSavedSearches(ctx, in.Body.IDs); err != nil {
				return nil, apiError(err)
			}
			return &emptyOutput{}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "get-saved-search", Method: http.MethodGet, Path: "/api/saved-searches/{id}", Summary: "Get a saved search"},
		func(ctx context.Context, in *idInput) (*savedSearchOutput, error) {
			b, err := svc.SavedSearch(ctx, in.ID)
			if err != nil {
				return nil, apiError(err)
			}
			return &savedSearchOutput{Body: toSavedSearch(b)}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "update-saved-search", Method: http.MethodPut, Path: "/api/saved-searches/{id}", Summary: "Replace the editable fields of a saved search"},
		func(ctx context.Context, in *updateSavedSearchInput) (*savedSearchOutput, error) {
			// Check existence first: SaveSavedSearch with an unknown ID must be a 404.
			if _, err := svc.SavedSearch(ctx, in.ID); err != nil {
				return nil, apiError(err)
			}
			b, err := svc.SaveSavedSearch(ctx, in.Body.savedSearch(in.ID))
			if err != nil {
				return nil, apiError(err)
			}
			return &savedSearchOutput{Body: toSavedSearch(b)}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "delete-saved-search", Method: http.MethodDelete, Path: "/api/saved-searches/{id}", Summary: "Delete a saved search", DefaultStatus: http.StatusNoContent},
		func(ctx context.Context, in *idInput) (*emptyOutput, error) {
			if err := svc.DeleteSavedSearch(ctx, in.ID); err != nil {
				return nil, apiError(err)
			}
			return &emptyOutput{}, nil
		})
}
