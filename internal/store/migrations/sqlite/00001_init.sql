-- +goose Up
CREATE TABLE bookmarks (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    url            TEXT NOT NULL,
    url_normalized TEXT NOT NULL UNIQUE,
    title          TEXT NOT NULL DEFAULT '',
    description    TEXT NOT NULL DEFAULT '',
    notes          TEXT NOT NULL DEFAULT '',
    unread         INTEGER NOT NULL DEFAULT 0,
    is_archived    INTEGER NOT NULL DEFAULT 0,
    favicon_file   TEXT NOT NULL DEFAULT '',
    preview_file   TEXT NOT NULL DEFAULT '',
    web_archive_url TEXT NOT NULL DEFAULT '',
    date_added     TEXT NOT NULL,
    date_modified  TEXT NOT NULL
);
CREATE INDEX idx_bookmarks_date_added ON bookmarks(date_added);
CREATE INDEX idx_bookmarks_date_modified ON bookmarks(date_modified);

CREATE TABLE tags (
    id   INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE
);

CREATE TABLE bookmark_tags (
    bookmark_id INTEGER NOT NULL REFERENCES bookmarks(id) ON DELETE CASCADE,
    tag_id      INTEGER NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    PRIMARY KEY (bookmark_id, tag_id)
);
CREATE INDEX idx_bookmark_tags_tag ON bookmark_tags(tag_id);

CREATE TABLE archives (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    bookmark_id  INTEGER NOT NULL REFERENCES bookmarks(id) ON DELETE CASCADE,
    source       TEXT NOT NULL CHECK (source IN ('extension','server','upload','import')),
    content_type TEXT NOT NULL DEFAULT '',
    file_name    TEXT NOT NULL DEFAULT '',
    display_name TEXT NOT NULL DEFAULT '',
    file_size    INTEGER NOT NULL DEFAULT 0,
    created_at   TEXT NOT NULL
);
CREATE INDEX idx_archives_bookmark ON archives(bookmark_id);

CREATE TABLE reader_texts (
    bookmark_id  INTEGER PRIMARY KEY REFERENCES bookmarks(id) ON DELETE CASCADE,
    archive_id   INTEGER REFERENCES archives(id) ON DELETE SET NULL,
    status       TEXT NOT NULL CHECK (status IN ('ok','failed','none')),
    title        TEXT NOT NULL DEFAULT '',
    markdown     TEXT NOT NULL DEFAULT '',
    error        TEXT NOT NULL DEFAULT '',
    word_count   INTEGER NOT NULL DEFAULT 0,
    extracted_at TEXT NOT NULL
);

CREATE TABLE saved_searches (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    name          TEXT NOT NULL,
    query         TEXT NOT NULL DEFAULT '',
    any_tags      TEXT NOT NULL DEFAULT '',
    all_tags      TEXT NOT NULL DEFAULT '',
    excluded_tags TEXT NOT NULL DEFAULT '',
    filter_unread TEXT NOT NULL DEFAULT 'off',
    sort_order    INTEGER NOT NULL DEFAULT 0,
    date_created  TEXT NOT NULL,
    date_modified TEXT NOT NULL
);

CREATE TABLE settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE jobs (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    kind        TEXT NOT NULL,
    bookmark_id INTEGER NOT NULL REFERENCES bookmarks(id) ON DELETE CASCADE,
    status      TEXT NOT NULL CHECK (status IN ('pending','running','done','failed')),
    attempts    INTEGER NOT NULL DEFAULT 0,
    run_at      TEXT NOT NULL,
    last_error  TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);
CREATE INDEX idx_jobs_status_run_at ON jobs(status, run_at);

-- Full-text index; rowid = bookmark id. Rows are written by the store's reindex
-- (no triggers: tags and reader text live in other tables).
CREATE VIRTUAL TABLE bookmarks_fts USING fts5(title, description, notes, url, tags, reader, tokenize='unicode61 remove_diacritics 2');

-- +goose Down
DROP TABLE bookmarks_fts;
DROP TABLE jobs;
DROP TABLE settings;
DROP TABLE saved_searches;
DROP TABLE reader_texts;
DROP TABLE archives;
DROP TABLE bookmark_tags;
DROP TABLE tags;
DROP TABLE bookmarks;
