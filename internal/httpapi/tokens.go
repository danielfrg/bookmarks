package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/danielfrg/bookmarks/internal/service"
	"github.com/danielgtaylor/huma/v2"
)

// tokenJSON never carries the secret or its hash.
type tokenJSON struct {
	ID         int64      `json:"id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix" doc:"Start of the token after bk_, for recognizing it"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at" nullable:"true"`
}

type createdTokenJSON struct {
	tokenJSON
	Token string `json:"token" doc:"The full token. Returned only by this request; store it now."`
}

func toToken(t service.TokenInfo) tokenJSON {
	out := tokenJSON{ID: t.ID, Name: t.Name, Prefix: t.Prefix, CreatedAt: t.CreatedAt.UTC()}
	if t.LastUsedAt != nil {
		u := t.LastUsedAt.UTC()
		out.LastUsedAt = &u
	}
	return out
}

type tokenListOutput struct{ Body []tokenJSON }

type createdTokenOutput struct{ Body createdTokenJSON }

type createTokenInput struct {
	Body struct {
		_    struct{} `additionalProperties:"true"`
		Name string   `json:"name" doc:"What the token is for, e.g. Chrome extension"`
	}
}

func registerTokens(api huma.API, svc *service.Service) {
	huma.Register(api, huma.Operation{OperationID: "list-tokens", Method: http.MethodGet, Path: "/api/tokens", Summary: "List API tokens, newest first"},
		func(ctx context.Context, _ *struct{}) (*tokenListOutput, error) {
			list, err := svc.ListTokens(ctx)
			if err != nil {
				return nil, apiError(err)
			}
			out := &tokenListOutput{Body: make([]tokenJSON, 0, len(list))}
			for _, t := range list {
				out.Body = append(out.Body, toToken(t))
			}
			return out, nil
		})

	huma.Register(api, huma.Operation{OperationID: "create-token", Method: http.MethodPost, Path: "/api/tokens", Summary: "Create an API token", Description: "The response is the only time the full token is shown.", DefaultStatus: http.StatusCreated},
		func(ctx context.Context, in *createTokenInput) (*createdTokenOutput, error) {
			info, plain, err := svc.CreateToken(ctx, in.Body.Name)
			if err != nil {
				return nil, apiError(err)
			}
			return &createdTokenOutput{Body: createdTokenJSON{tokenJSON: toToken(info), Token: plain}}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "delete-token", Method: http.MethodDelete, Path: "/api/tokens/{id}", Summary: "Delete (revoke) an API token", DefaultStatus: http.StatusNoContent},
		func(ctx context.Context, in *idInput) (*emptyOutput, error) {
			if err := svc.DeleteToken(ctx, in.ID); err != nil {
				return nil, apiError(err)
			}
			return &emptyOutput{}, nil
		})
}
