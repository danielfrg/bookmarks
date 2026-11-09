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

## Run

```sh
just api
```

The server listens on `127.0.0.1:8080` and keeps its data (SQLite database,
favicons, previews, archives) in `./data`. API docs are at `/_/api/docs` and the
MCP endpoint is `/_/mcp`.

## Using the web UI

Open the server's root (`http://127.0.0.1:8080`). Search, filter, sort, page and
open dialogs are all in the URL, so reloads and back/forward work.

**Search syntax**

| Query | Meaning |
| --- | --- |
| `#tag` | Has the tag |
| `!unread` | Unread bookmarks |
| `!untagged` | Bookmarks without tags |
| `!basic` | Basic captures (no full archive) |
| `"exact phrase"` | Phrase match |
| `foo and bar`, `foo or bar`, `not foo` | Boolean operators |

**Keyboard shortcuts** (on the list, with no dialog open; `?` shows them in the app)

| Key | Action |
| --- | --- |
| `n` | Add a bookmark |
| `s` or `/` | Focus search |
| `j` / `k` or `↓` / `↑` | Move the highlighted row |
| `Enter` or `o` | Open the highlighted link in a new tab |
| `v` | View details |
| `e` | Edit |

## Configuration

All settings are environment variables.

| Name | Default | Meaning |
| --- | --- | --- |
| `BOOKMARKS_ADDR` | `127.0.0.1:8080` | Address to listen on |
| `BOOKMARKS_DATA_DIR` | `./data` | Data directory; the database is `<DATA_DIR>/bookmarks.db` |
| `BOOKMARKS_PUBLIC_URL` | `http://127.0.0.1:8080` | Public origin of the app (no path) |
| `BOOKMARKS_API_TOKEN` | unset | Token accepted as `Authorization: Bearer <token>` by the API and MCP |
| `BOOKMARKS_API_TOKEN_FILE` | unset | Read the API token from this file (not together with `API_TOKEN`) |
| `BOOKMARKS_MCP_ALLOWED_HOSTS` | unset | Extra comma-separated hostnames the MCP endpoint accepts (localhost is always allowed) |
| `BOOKMARKS_MAX_UPLOAD_MB` | `100` | Maximum uncompressed archive size, in MB |
| `BOOKMARKS_FAVICON_PROVIDER` | Google favicon service | Favicon URL template containing `{url}` |
| `BOOKMARKS_ALLOW_PRIVATE_FETCH` | `false` | Let the server fetch private/loopback addresses |

## Endpoints

- `/_/api/docs`: interactive API documentation (OpenAPI at `/_/api/openapi`)
- `/_/mcp`: MCP server (search and read bookmarks)
- `/_/healthz`, `/_/readyz`: health checks

## Development

Requires Go, Node, pnpm and [just](https://github.com/casey/just).
See the `justfile` for the available recipes.

```sh
just install   # frontend dependencies
just api       # Go API on 127.0.0.1:8080
just frontend  # Vite dev server on 127.0.0.1:5173, proxies /_/api etc. to the API
```

Set `BOOKMARKS_API_TARGET` to point the dev server at a different API address.
`just build` tests and builds the frontend, embeds it in `internal/web/dist` and builds the `bookmarks` binary.
