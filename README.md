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
| `cmd/bookmarks` | The server: web UI, REST API (`/api`), MCP (`/mcp`) |
| `cmd/import-linkding` | One-off migration from a linkding instance, archives included |
| `frontend/` | Svelte 5 web UI |
| `extension/` | Chrome extension |

## Run

### Docker

```zsh
docker run -it \
  -p 127.0.0.1:8080:8080 \
  -v bookmarks-data:/data \
  -e BOOKMARKS_PUBLIC_URL=http://localhost:8080 \
  ghcr.io/danielfrg/bookmarks:latest
```

- Open [http://localhost:8080](http://localhost:8080)
- The SQLite database, favicons, previews and archives are stored in `/data`
- The container runs as UID/GID 65532

Published images support Linux AMD64 and ARM64. `latest` tracks `main`.

### From source

```sh
just api
```

The server listens on `127.0.0.1:8080` and keeps its data (SQLite database,
favicons, previews, archives) in `./data`. API docs are at `/api/docs` and the
MCP endpoint is `/mcp`.

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
| `!noarchive` | Bookmarks with no archive at all |
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
| `BOOKMARKS_MCP_ALLOWED_HOSTS` | unset | Extra comma-separated hostnames the MCP endpoint accepts (localhost is always allowed) |
| `BOOKMARKS_MAX_UPLOAD_MB` | `100` | Maximum uncompressed archive size, in MB |
| `BOOKMARKS_FAVICON_PROVIDER` | Google favicon service | Favicon URL template containing `{url}` |
| `BOOKMARKS_ALLOW_PRIVATE_FETCH` | `false` | Let the server fetch private/loopback addresses |

## Endpoints

- `/api/docs`: interactive API documentation (OpenAPI at `/api/openapi`)
- `/mcp`: MCP server (search and read bookmarks)
- `/healthz`, `/readyz`: health checks

## API tokens

Create API tokens in the web UI under **Settings → API**: give each app its own
named token (the Chrome extension, `import-linkding`, an agent). The full token
(`bk_…`) is shown once, when it is created; the server stores only its hash.
Clients send it as `Authorization: Bearer <token>` (linkding's
`Authorization: Token <token>` also works). Deleting a token revokes it
immediately. Tokens can only be managed from the web UI: `/api/tokens`
rejects requests that carry a token.

The web UI itself needs no token (put the app behind your own access control).
MCP is open on its allowed hosts; a request that does send a token must send a
valid one.

## Migrating from linkding

`cmd/import-linkding` copies everything from a linkding instance through both
REST APIs: bookmarks (active and archived, with their dates, tags, notes and
read state), complete snapshots and uploaded files as archives (keeping their
creation dates) and bundles as saved searches. `BOOKMARKS_TOKEN` is an API
token created in Settings → API. Do a dry run first:

```sh
export LINKDING_TOKEN=... BOOKMARKS_TOKEN=...
go run ./cmd/import-linkding \
  --linkding-url https://linkding.example.com \
  --bookmarks-url https://bookmarks.example.com \
  --dry-run
# then the same command without --dry-run
```

`--limit N` imports only the first N bookmarks and `--skip-assets` skips the
archives. It prints one line per bookmark and exits 1 if anything failed.
Re-running is safe and is how failures are retried: bookmarks merge by URL,
archives already copied are skipped and saved searches are matched by name.

## Development

Requires Go, Node, pnpm and [just](https://github.com/casey/just).
See the `justfile` for the available recipes.

```sh
just install   # frontend dependencies
just api       # Go API on 127.0.0.1:8080
just frontend  # Vite dev server on 127.0.0.1:5173, proxies /api etc. to the API
```

Set `BOOKMARKS_API_TARGET` to point the dev server at a different API address.
`just build` tests and builds the frontend, embeds it in `internal/web/dist` and builds the `bookmarks` binary.
