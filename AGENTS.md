# Agent Instructions

Bookmarks is a self-hosted, single-user bookmark manager.
One Go binary serves a Huma REST API, an MCP endpoint, and an embedded Svelte 5
frontend backed by SQLite. A Chrome extension
in `/extension` saves bookmarks and uploads full-page archives.

Read `PRODUCT.md` for user intent and `DESIGN.md` before changing the frontend.
The `README.md` and `justfile` are for humans: keep them current when changes are
user facing, avoid big restructures.

## Architecture

- `frontend/src/design-system/` and `extension/src/design-system/` are synced from the shared design system and are never edited by hand (change it there and run its sync); the Cadence theme and the ui-sdk components are described in `DESIGN.md`.
- Go in `/cmd` and `/internal`; TypeScript + Svelte 5 in `/frontend` (components in
  `frontend/src/lib/components`); the extension (WXT + Svelte 5) in `/extension`.

### Vocabulary

- **Archive**: a saved full-page file (single-file HTML, PDF, or upload). Each
  has a source: `extension`, `server`, `upload`, or `import` (archives from a
  one-time migration; nothing creates new ones). There is no "archived
  bookmark" state; archive always means the file.
- **Basic capture**: a `server`-source archive (Go fetch, no JavaScript). A
  bookmark with only basic captures shows a badge until the extension adds a full one.
- **Reader text**: markdown extracted from the latest archive. Separate from
  archives, stored, indexed, and re-extractable.

## Behavior Invariants

- Keep SQLite rollback journaling (no WAL); the database lives on NFS.
- Keep SQL behind the store interface and full-text search behind its own
  interface; Postgres may replace SQLite later.
- Routes: the REST API lives under `/api`, MCP at `/mcp`, files under `/archives`, `/favicons` and `/previews`, built frontend assets under `/assets`, and health checks at `/healthz` and `/readyz`. The app pages are `/`, `/tags`, `/saved` and `/settings/{section}` (listed in `internal/httpapi/handler.go` and `frontend/src/lib/view.ts`); no SPA catch-all, unknown paths are 404.
- The web UI has no auth (it runs behind Cloudflare Access / internal network).
  External API clients send an API token (`Bearer`/`Token`) created in Settings →
  API; only its SHA-256 is stored and the plaintext is shown once. `/api/tokens`
  is browser-only (bearer requests get 403). MCP is open on
  its allowed hosts, but a token it is sent must verify.
- MCP and the HTTP API share one service layer and validation.
- Agents get reader text (paged markdown), never raw archive HTML.
- Server-side fetches must block non-public addresses.
- Archive files live on disk (gzipped) in the data directory, not in the database.

## Development

Requires Go, Node and pnpm; use the `just` recipes:

- `just install`: install the frontend dependencies.
- `just api`: Go API on `127.0.0.1:8080` (serves the frontend embedded by the last `just build`).
- `just frontend`: Vite dev server with HMR on `127.0.0.1:5173`, proxies the API.
- `just check`: gofmt, vet, race tests, oxfmt/oxlint, frontend tests and svelte-check.
- `just build`: build the frontend, embed it and build the single binary.
- `just run`: build, then run the binary.
- `just image-check`: build the Docker image and run `scripts/container-smoke.sh` against it.
- Extension: `just ext-install`, `just ext-dev`, `just ext-build`, `just ext-zip`.

## Frontend Conventions

- Read `DESIGN.md` before changing the frontend.
- `frontend/src/design-system/` is synced from the design-system repo with `pnpm sync`; never edit it.
- `main.ts` imports the synced design-system CSS first, then `palette.css` if any, then
  `app.css` last, so app rules win ties with the theme; prefer tokens and class selectors
  over `!important`. `app.css` keeps `@layer ui-sdk;` as its first statement. This app has no
  `palette.css`; the extension's `lib/boot.ts` follows the same order.
- Components live in `frontend/src/lib/components`; `App.svelte` stays at the top of `frontend/src`.
