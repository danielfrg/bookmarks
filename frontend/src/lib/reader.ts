/** Characters requested per reader page. */
export const READER_PAGE_CHARS = 20000;

/**
 * Appends the next reader page. The server cuts pages on paragraph breaks, so
 * the break is usually already consumed; add one when the text so far does not
 * end in a newline so paragraphs do not merge.
 */
export function joinChunks(prev: string, next: string): string {
  if (prev === "") return next;
  if (next === "") return prev;
  return prev.endsWith("\n") ? prev + next : `${prev}\n\n${next}`;
}
