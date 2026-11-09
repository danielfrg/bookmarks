package mcp

// What the server says about itself when a client connects. Agents pick the
// moment to use a tool from this text and the tool descriptions.
const serverInstructions = `Bookmarks is Daniel's personal bookmark library with saved copies of each page. Use search_bookmarks to find pages on a topic; it searches titles, tags, notes and the full saved text. Search is keyword-based: when results are thin, try several phrasings, synonyms and related terms before concluding nothing is saved. Read the few most relevant results with read_bookmark rather than every hit, and page through long texts with next_offset. Query syntax: #tag, !unread, !untagged, "exact phrase", and/or/not. Use add_bookmark only when asked to save a link.`
