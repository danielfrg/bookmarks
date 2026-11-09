package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/danielfrg/bookmarks/internal/bookmarks"
	"github.com/danielfrg/bookmarks/internal/service"
	"github.com/danielgtaylor/huma/v2"
)

type bundleJSON struct {
	ID           int64     `json:"id"`
	Name         string    `json:"name"`
	Search       string    `json:"search"`
	AnyTags      []string  `json:"any_tags"`
	AllTags      []string  `json:"all_tags"`
	ExcludedTags []string  `json:"excluded_tags"`
	FilterUnread string    `json:"filter_unread"`
	Order        int       `json:"order"`
	DateCreated  time.Time `json:"date_created"`
	DateModified time.Time `json:"date_modified"`
}

func toBundle(b bookmarks.Bundle) bundleJSON {
	nonNil := func(s []string) []string {
		if s == nil {
			return []string{}
		}
		return s
	}
	return bundleJSON{
		ID: b.ID, Name: b.Name, Search: b.Search,
		AnyTags: nonNil(b.AnyTags), AllTags: nonNil(b.AllTags), ExcludedTags: nonNil(b.ExcludedTags),
		FilterUnread: b.FilterUnread, Order: b.Order,
		DateCreated: b.DateCreated.UTC(), DateModified: b.DateModified.UTC(),
	}
}

// bundleBody is the editable part of a bundle. Everything but the name is
// optional; filter_unread is validated by the service.
type bundleBody struct {
	_            struct{} `additionalProperties:"true"`
	Name         string   `json:"name"`
	Search       string   `json:"search,omitempty"`
	AnyTags      []string `json:"any_tags,omitempty"`
	AllTags      []string `json:"all_tags,omitempty"`
	ExcludedTags []string `json:"excluded_tags,omitempty"`
	FilterUnread string   `json:"filter_unread,omitempty"`
}

func (b bundleBody) bundle(id int64) bookmarks.Bundle {
	return bookmarks.Bundle{
		ID: id, Name: b.Name, Search: b.Search, AnyTags: b.AnyTags, AllTags: b.AllTags,
		ExcludedTags: b.ExcludedTags, FilterUnread: b.FilterUnread,
	}
}

type bundleOutput struct{ Body bundleJSON }

type bundleListOutput struct{ Body []bundleJSON }

type createBundleInput struct{ Body bundleBody }

type updateBundleInput struct {
	ID   int64 `path:"id" minimum:"1"`
	Body bundleBody
}

type reorderBundlesInput struct {
	Body struct {
		_   struct{} `additionalProperties:"true"`
		IDs []int64  `json:"ids"`
	}
}

func registerBundles(api huma.API, svc *service.Service) {
	huma.Register(api, huma.Operation{OperationID: "list-bundles", Method: http.MethodGet, Path: "/_/api/bundles", Summary: "List bundles in display order"},
		func(ctx context.Context, _ *struct{}) (*bundleListOutput, error) {
			list, err := svc.Bundles(ctx)
			if err != nil {
				return nil, apiError(err)
			}
			out := &bundleListOutput{Body: make([]bundleJSON, 0, len(list))}
			for _, b := range list {
				out.Body = append(out.Body, toBundle(b))
			}
			return out, nil
		})

	huma.Register(api, huma.Operation{OperationID: "create-bundle", Method: http.MethodPost, Path: "/_/api/bundles", Summary: "Create a bundle", DefaultStatus: http.StatusCreated},
		func(ctx context.Context, in *createBundleInput) (*bundleOutput, error) {
			b, err := svc.SaveBundle(ctx, in.Body.bundle(0))
			if err != nil {
				return nil, apiError(err)
			}
			return &bundleOutput{Body: toBundle(b)}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "reorder-bundles", Method: http.MethodPost, Path: "/_/api/bundles/reorder", Summary: "Set the bundle order", DefaultStatus: http.StatusNoContent},
		func(ctx context.Context, in *reorderBundlesInput) (*emptyOutput, error) {
			if err := svc.ReorderBundles(ctx, in.Body.IDs); err != nil {
				return nil, apiError(err)
			}
			return &emptyOutput{}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "get-bundle", Method: http.MethodGet, Path: "/_/api/bundles/{id}", Summary: "Get a bundle"},
		func(ctx context.Context, in *idInput) (*bundleOutput, error) {
			b, err := svc.Bundle(ctx, in.ID)
			if err != nil {
				return nil, apiError(err)
			}
			return &bundleOutput{Body: toBundle(b)}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "update-bundle", Method: http.MethodPut, Path: "/_/api/bundles/{id}", Summary: "Replace the editable fields of a bundle"},
		func(ctx context.Context, in *updateBundleInput) (*bundleOutput, error) {
			// Check existence first: SaveBundle with an unknown ID must be a 404.
			if _, err := svc.Bundle(ctx, in.ID); err != nil {
				return nil, apiError(err)
			}
			b, err := svc.SaveBundle(ctx, in.Body.bundle(in.ID))
			if err != nil {
				return nil, apiError(err)
			}
			return &bundleOutput{Body: toBundle(b)}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "delete-bundle", Method: http.MethodDelete, Path: "/_/api/bundles/{id}", Summary: "Delete a bundle", DefaultStatus: http.StatusNoContent},
		func(ctx context.Context, in *idInput) (*emptyOutput, error) {
			if err := svc.DeleteBundle(ctx, in.ID); err != nil {
				return nil, apiError(err)
			}
			return &emptyOutput{}, nil
		})
}
