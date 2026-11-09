# Bookmarks

A self-hosted, single-user bookmark manager inspired by
[linkding](https://github.com/sissbruecker/linkding).

> Work in progress.

- Save bookmarks with tags, notes, and unread/archived state.
- One Chrome extension saves the bookmark **and** a full-page archive
  (SingleFile-style) of the page as you see it.
- Every archive gets a stored reader-text version, indexed for full-text search.
- REST API and an MCP server, so agents can search and read your bookmarks.
- SQLite storage; one Go binary with the web UI embedded.

## Components

| Path | What |
| --- | --- |
| `cmd/bookmarks` | The server: web UI, REST API (`/_/api`), MCP (`/_/mcp`) |
| `cmd/import-linkding` | One-off migration from a linkding instance, archives included |
| `frontend/` | Svelte 5 web UI |
| `extension/` | Chrome extension |

## Development

Requires Go, Node, pnpm and [just](https://github.com/casey/just).
See the `justfile` for the available recipes.
