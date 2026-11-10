# Bookmarks extension

Chrome (Manifest V3) extension that saves the current page to your Bookmarks
server, optionally with a full-page archive captured in the browser.

Built with WXT, Svelte 5 and kit-ui. `src/theme.css` and `src/components.css`
are byte-identical copies of the shared design system.

## Develop

```sh
just ext-install
just ext-dev     # then load .output/chrome-mv3 via chrome://extensions
just ext-build   # test, check, build
just ext-zip
```

Settings (server URL, API token, unread default) live in
`chrome.storage.local`.

## License note

`single-file-core` is AGPL-3.0. This extension is for personal use and is not
published.
