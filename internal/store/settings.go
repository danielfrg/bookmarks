package store

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/danielfrg/bookmarks/internal/bookmarks"
)

// GetSettings starts from the defaults and overlays any stored keys.
func (s *SQLite) GetSettings(ctx context.Context) (bookmarks.Settings, error) {
	v := bookmarks.DefaultSettings()
	rows, err := s.db.QueryContext(ctx, `SELECT key,value FROM settings`)
	if err != nil {
		return v, err
	}
	defer rows.Close()
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return v, err
		}
		switch key {
		case "favicons_enabled":
			err = json.Unmarshal([]byte(value), &v.FaviconsEnabled)
		case "previews_enabled":
			err = json.Unmarshal([]byte(value), &v.PreviewsEnabled)
		case "wayback_enabled":
			err = json.Unmarshal([]byte(value), &v.WaybackEnabled)
		case "default_unread":
			err = json.Unmarshal([]byte(value), &v.DefaultUnread)
		case "auto_tag_rules":
			err = json.Unmarshal([]byte(value), &v.AutoTagRules)
		case "ui":
			v.UI = json.RawMessage(value)
		}
		if err != nil {
			return v, err
		}
	}
	return v, rows.Err()
}

// PutSettings stores one row per key, JSON-encoded.
func (s *SQLite) PutSettings(ctx context.Context, v bookmarks.Settings) error {
	ui := v.UI
	if len(ui) == 0 {
		ui = json.RawMessage(`{}`)
	}
	autoTags, err := json.Marshal(v.AutoTagRules)
	if err != nil {
		return err
	}
	values := [][2]string{
		{"favicons_enabled", strconv.FormatBool(v.FaviconsEnabled)},
		{"previews_enabled", strconv.FormatBool(v.PreviewsEnabled)},
		{"wayback_enabled", strconv.FormatBool(v.WaybackEnabled)},
		{"default_unread", strconv.FormatBool(v.DefaultUnread)},
		{"auto_tag_rules", string(autoTags)},
		{"ui", string(ui)},
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, kv := range values {
		if _, err := tx.ExecContext(ctx, `INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, kv[0], kv[1]); err != nil {
			return err
		}
	}
	return tx.Commit()
}
