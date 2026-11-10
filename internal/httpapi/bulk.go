package httpapi

import (
	"context"
	"net/http"

	"github.com/danielfrg/bookmarks/internal/service"
	"github.com/danielgtaylor/huma/v2"
)

type bulkInput struct {
	Body struct {
		_     struct{} `additionalProperties:"true"`
		IDs   []int64  `json:"ids,omitempty"`
		Query *struct {
			_      struct{} `additionalProperties:"true"`
			Q      string   `json:"q,omitempty"`
			Unread string   `json:"unread,omitempty"`
		} `json:"query,omitempty" doc:"Used when ids is empty"`
		Action string   `json:"action" doc:"delete, tag, untag, read, unread, refresh_metadata, capture or extract"`
		Tags   []string `json:"tags,omitempty" doc:"For the tag and untag actions"`
	}
}

type bulkOutput struct {
	Body struct {
		Affected int `json:"affected"`
	}
}

func registerBulk(api huma.API, svc *service.Service) {
	huma.Register(api, huma.Operation{OperationID: "bulk-bookmarks", Method: http.MethodPost, Path: "/api/bookmarks/bulk", Summary: "Apply an action to many bookmarks"},
		func(ctx context.Context, in *bulkInput) (*bulkOutput, error) {
			bi := service.BulkInput{IDs: in.Body.IDs, Action: in.Body.Action, Tags: in.Body.Tags}
			if q := in.Body.Query; q != nil {
				bi.Query = &service.SearchInput{Q: q.Q, Unread: q.Unread}
			}
			n, err := svc.Bulk(ctx, bi)
			if err != nil {
				return nil, apiError(err)
			}
			out := &bulkOutput{}
			out.Body.Affected = n
			return out, nil
		})
}
