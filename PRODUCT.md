# Product

## Users

One person (Daniel) saving links from the browser, and the agents he works with.
Assumes deployment on a trusted network or behind Cloudflare Access; there are
no accounts.

## Product Purpose

Save a page in one click and keep it forever: the bookmark, a full-page archive
as it looked in the browser, and clean reader text.

Find it again later, by browsing tags and searching in the UI, or by asking an
agent ("find my bookmarks about public speaking and apply them to these slides").

Success means one extension that never needs re-wiring, archives that survive
the original page disappearing, and agents that can answer from saved content.

## Brand Personality

Simple, dense, quiet. Like linkding: the list of bookmarks is the interface.

### Tone

Don't explain every part of the UI with text. Add messages only when needed
(errors, a basic capture waiting for a full one, an expired Cloudflare session).

## Anti-references

- An extension that depends on another extension or needs more setup than a
  server URL and a token.
- Losing archives, including those migrated from linkding.
- Drifting from linkding's simple structure: no dashboards, no sidebar navigation.
- Handing agents raw HTML or unbounded text.

## Design Principles

- The bookmark list comes first; filters, bundles and tags sit beside it.
- Saving an existing URL updates the bookmark instead of duplicating it.
- Archives and reader text are visible and recoverable: open, download,
  capture again, re-extract.
- Distinguish loading, empty, no results, and request failures.

## Agent Access

Agents use typed MCP tools: search (with snippets), read a bookmark's reader
text in pages, list tags, and add bookmarks. Agents should try several phrasings
when a search comes back thin.
