/**
 * Commits typed tag text: splits on whitespace/commas, lower-cases, drops a
 * leading "#" and empty parts, and appends tags not already present.
 * Same rules as the web app's commitTagText.
 */
export function commitTagText(current: string[], text: string): string[] {
  const result = [...current];
  for (const raw of text.split(/[\s,]+/)) {
    const tag = raw.replace(/^#+/, "").toLowerCase();
    if (tag && !result.includes(tag)) result.push(tag);
  }
  return result;
}
