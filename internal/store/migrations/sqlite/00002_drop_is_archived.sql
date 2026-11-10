-- "Archive" means a saved page file (the archives table) only; bookmarks no
-- longer carry a linkding-style archived flag.
-- +goose Up
ALTER TABLE bookmarks DROP COLUMN is_archived;

-- +goose Down
ALTER TABLE bookmarks ADD COLUMN is_archived INTEGER NOT NULL DEFAULT 0;
