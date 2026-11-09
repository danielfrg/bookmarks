package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielfrg/bookmarks/internal/bookmarks"
	"github.com/danielfrg/bookmarks/internal/service"
	"github.com/danielgtaylor/huma/v2"
)

var errTagVanished = errors.New("renamed tag not found")

type tagJSON struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type tagOutput struct{ Body tagJSON }

type tagListOutput struct{ Body []tagJSON }

type renameTagInput struct {
	ID   int64 `path:"id" minimum:"1"`
	Body struct {
		_    struct{} `additionalProperties:"true"`
		Name string   `json:"name" minLength:"1"`
	}
}

func registerTags(api huma.API, svc *service.Service) {
	huma.Register(api, huma.Operation{OperationID: "list-tags", Method: http.MethodGet, Path: "/api/tags", Summary: "List tags with their bookmark counts"},
		func(ctx context.Context, _ *struct{}) (*tagListOutput, error) {
			tags, err := svc.Tags(ctx)
			if err != nil {
				return nil, apiError(err)
			}
			out := &tagListOutput{Body: make([]tagJSON, 0, len(tags))}
			for _, t := range tags {
				out.Body = append(out.Body, tagJSON{ID: t.ID, Name: t.Name, Count: t.Count})
			}
			return out, nil
		})

	huma.Register(api, huma.Operation{OperationID: "rename-tag", Method: http.MethodPatch, Path: "/api/tags/{id}", Summary: "Rename a tag",
		Description: "Renaming to the name of an existing tag merges the two. Returns the resulting tag."},
		func(ctx context.Context, in *renameTagInput) (*tagOutput, error) {
			if err := svc.RenameTag(ctx, in.ID, in.Body.Name); err != nil {
				return nil, apiError(err)
			}
			tags, err := svc.Tags(ctx)
			if err != nil {
				return nil, apiError(err)
			}
			want := bookmarks.NormalizeTags([]string{in.Body.Name})
			for _, t := range tags {
				if t.ID == in.ID || (len(want) == 1 && t.Name == want[0]) {
					return &tagOutput{Body: tagJSON{ID: t.ID, Name: t.Name, Count: t.Count}}, nil
				}
			}
			return nil, apiError(errTagVanished)
		})

	huma.Register(api, huma.Operation{OperationID: "delete-tag", Method: http.MethodDelete, Path: "/api/tags/{id}", Summary: "Delete a tag", DefaultStatus: http.StatusNoContent},
		func(ctx context.Context, in *idInput) (*emptyOutput, error) {
			if err := svc.DeleteTag(ctx, in.ID); err != nil {
				return nil, apiError(err)
			}
			return &emptyOutput{}, nil
		})
}
