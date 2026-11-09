package httpapi

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/danielfrg/bookmarks/internal/bookmarks"
	"github.com/danielfrg/bookmarks/internal/service"
	"github.com/danielgtaylor/huma/v2"
)

type settingsJSON struct {
	FaviconsEnabled bool            `json:"favicons_enabled"`
	PreviewsEnabled bool            `json:"previews_enabled"`
	WaybackEnabled  bool            `json:"wayback_enabled"`
	DefaultUnread   bool            `json:"default_unread"`
	AutoTagRules    string          `json:"auto_tag_rules"`
	UI              json.RawMessage `json:"ui" type:"object" doc:"Arbitrary UI preferences (JSON object)"`
}

type settingsOutput struct{ Body settingsJSON }

type putSettingsInput struct {
	Body struct {
		_               struct{}        `additionalProperties:"true"`
		FaviconsEnabled bool            `json:"favicons_enabled,omitempty"`
		PreviewsEnabled bool            `json:"previews_enabled,omitempty"`
		WaybackEnabled  bool            `json:"wayback_enabled,omitempty"`
		DefaultUnread   bool            `json:"default_unread,omitempty"`
		AutoTagRules    string          `json:"auto_tag_rules,omitempty"`
		UI              json.RawMessage `json:"ui,omitempty" type:"object"`
	}
}

func toSettings(s bookmarks.Settings) settingsJSON {
	ui := s.UI
	if len(ui) == 0 {
		ui = json.RawMessage(`{}`)
	}
	return settingsJSON{
		FaviconsEnabled: s.FaviconsEnabled, PreviewsEnabled: s.PreviewsEnabled, WaybackEnabled: s.WaybackEnabled,
		DefaultUnread: s.DefaultUnread, AutoTagRules: s.AutoTagRules, UI: ui,
	}
}

func registerSettings(api huma.API, svc *service.Service) {
	huma.Register(api, huma.Operation{OperationID: "get-settings", Method: http.MethodGet, Path: "/_/api/settings", Summary: "Get the settings"},
		func(ctx context.Context, _ *struct{}) (*settingsOutput, error) {
			s, err := svc.Settings(ctx)
			if err != nil {
				return nil, apiError(err)
			}
			return &settingsOutput{Body: toSettings(s)}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "put-settings", Method: http.MethodPut, Path: "/_/api/settings", Summary: "Replace the settings",
		Description: "The whole settings object is replaced: omitted fields become false, empty or {}."},
		func(ctx context.Context, in *putSettingsInput) (*settingsOutput, error) {
			ui := json.RawMessage(`{}`)
			if len(in.Body.UI) > 0 {
				var obj map[string]json.RawMessage
				if err := json.Unmarshal(in.Body.UI, &obj); err != nil || obj == nil {
					return nil, huma.Error422UnprocessableEntity("ui: must be a JSON object")
				}
				ui = in.Body.UI
			}
			s, err := svc.PutSettings(ctx, bookmarks.Settings{
				FaviconsEnabled: in.Body.FaviconsEnabled, PreviewsEnabled: in.Body.PreviewsEnabled,
				WaybackEnabled: in.Body.WaybackEnabled, DefaultUnread: in.Body.DefaultUnread,
				AutoTagRules: in.Body.AutoTagRules, UI: ui,
			})
			if err != nil {
				return nil, apiError(err)
			}
			return &settingsOutput{Body: toSettings(s)}, nil
		})
}
