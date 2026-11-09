# Design System

Bookmarks shares the look of Daniel's other self-hosted apps and the layout of
linkding. Use a small, consistent vocabulary instead of one-off styling.

## Component Rules

- `frontend/src/theme.css` and `components.css` are copied byte-identical from
  `~/code/danielfrg/design-system/public/v1/` and kept identical across apps.
  Changes there must be generic and minimal. App rules go in `frontend/src/app.css`.
- Import shared controls directly from `@kenn-io/kit-ui`; icons from `@lucide/svelte`.
- Svelte 5 runes, typed props, callback props. Local components only for
  app-specific behavior, not pass-through wrappers.
- Inter for interface text; IBM Plex Mono for URLs, identifiers, and query syntax.
  Load both from Google Fonts with `display=swap`.
- Semantic theme tokens, default kit-ui control sizes, no Tailwind.
- The extension uses the same theme files and kit-ui.

## Appearance and Accessibility

Start in dark mode; respect explicit light or system preferences via `initTheme`
with the app's own storage key. Keep focus visible, controls labeled, and
reduced-motion preferences respected. Phone layouts must not widen the page.
Preserve `scrollbar-gutter: stable`.

## Application Patterns

Linkding's structure:

- **Main page:** no left sidebar. Top bar: wordmark, search (accepts the query
  syntax: `#tag`, `!unread`, `!untagged`, `!basic`, `and/or/not`, quotes),
  Active / Unread / Archived filter pills, Add. A dense bookmark list; a right
  panel with Saved searches, then an A–Z grouped tag cloud.
- **List row:** favicon, title (unread dot; clicking opens the live page), one-line
  description, tags, relative date, `archived` / `basic capture` badge, actions
  View, Edit, Archive, Delete.
- **Details modal (View):** title + URL, tags, description, an **Archive** section
  (saved snapshots: source, date, size, Open, Download; capture again, upload,
  Wayback link), a separate **Reader text** section (word count, Read,
  Re-extract), notes (markdown), unread toggle, dates; footer Edit / Archive /
  Delete.
- **Extension popup:** "already saved" banner (or "only a basic capture exists"),
  URL, title, tags with autocomplete, description, notes, "Archive this page" and
  "Mark unread" checkboxes, Save with capture progress.
- Search, filters and selection survive reload and browser history.
