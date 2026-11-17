# Design System

Bookmarks uses the shared design system. Use a small,
consistent vocabulary instead of one-off styling.

## Component Rules

- The look comes from the shared design system (Cadence theme), synced into
  `frontend/src/design-system/` (`theme/`, plus the `table` and `theme-toggle`
  components). That folder is never edited by hand: change it in the
  design-system repo and run `pnpm sync` there.
- Controls are `@ui-sdk/svelte` components (pinned exact, with `@ui-sdk/core`).
  Dialogs use `Dialog`/`AlertDialog`, file uploads use `FileInput`. ui-sdk has no
  Table, EmptyState or ThemeToggle: the table and toggle are shared, EmptyState,
  Spinner, Modal, Markdown and the like are small local components in
  `src/lib/components`. Icons from `@lucide/svelte`.
- `main.ts` imports the synced design-system CSS first, then `app.css` last, so app
  rules win ties with the theme (the extension's `lib/boot.ts` does the same). Prefer
  tokens and class selectors over `!important`.
- `app.css` starts with `@layer ui-sdk;` and holds only layout and app-specific
  pieces. It may adjust a control's layout (width, spacing), never its look. Use
  tokens (`--ink`, `--muted`, `--line`, `--space-*`, `--radius-*`, `--mono`).
  The theme has no global reset, so `app.css` sets `box-sizing` and `body` margin.
- Style custom wrappers with a class, not `data-slot` (ui-sdk's Button always sets
  `data-slot="button"`). Fields default to 20rem wide; set widths in `app.css`.
  A panel gets a 1px border or `--shadow-1`, never both; use `--shadow-drop`.
- Svelte 5 runes, typed props, callback props. No Tailwind.
- The theme's system font stack for the interface and its `--mono` for URLs,
  identifiers and query syntax. No web fonts are loaded and no font files are embedded.
- The extension uses the same Cadence theme, synced into `extension/src/design-system/`
  (it has no CSP-allowed inline script, so `lib/boot.ts` sets the saved scheme before mount).

## Appearance and Accessibility

Start in dark mode; a saved light, dark or system choice wins. The shared
`color-scheme` localStorage key is managed by `src/lib/color-scheme.ts` and the
synced `ThemeToggle`; a Vite plugin applies it before the app loads. Keep focus
visible, controls labeled, and reduced-motion preferences respected. Phone
layouts must not widen the page. Preserve `scrollbar-gutter: stable`.

## Application Patterns

Structure:

- **Main page:** no left sidebar. Top bar: wordmark, search (accepts the query
  syntax: `#tag`, `!unread`, `!untagged`, `!basic`, `!noarchive`, `and/or/not`, quotes),
  All / Unread filter pills (All is the default), Add, Select, sort, help. A dense
  bookmark list; a right
  panel with Saved searches, then an A–Z grouped tag cloud.
- **List row:** favicon, title (unread dot; clicking opens the live page), one-line
  description, tags, relative date, `archived` / `basic capture` badge (the
  bookmark has a saved page file), actions View (opens the newest archive),
  Details, Edit, Mark read (when unread), Delete. There is no archived-bookmark
  state: "archive" always means a saved page file.
- **Details modal (Details):** title + URL, tags, description, an **Archive** section
  (saved snapshots: source, date, size, Open, Download; capture again, upload,
  Wayback link), a separate **Reader text** section (word count, Read,
  Re-extract), notes (markdown), unread toggle, dates; footer Edit / Delete.
- **Extension popup:** "already saved" banner (or "only a basic capture exists"),
  URL, title, tags with autocomplete, description, notes, "Mark unread", and a main
  button with capture progress: Save on a new page (saves and archives; a failed
  archive still keeps the bookmark), Update on a saved one (metadata only) next to
  Archive / Re-archive.
- Search, filters and selection survive reload and browser history.
