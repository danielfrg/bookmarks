export type CapturabilityReason =
  | "browser-page"
  | "web-store"
  | "pdf"
  | "file-access"
  | "unsupported-scheme";

export type Capturability = { ok: true } | { ok: false; reason: CapturabilityReason };

const BROWSER_SCHEMES = new Set([
  "chrome:",
  "chrome-extension:",
  "chrome-untrusted:",
  "chrome-search:",
  "edge:",
  "about:",
  "view-source:",
  "devtools:",
]);

/**
 * Whether the extension can inject the capture script into a tab showing
 * `url`. Chrome forbids scripting browser pages and the Web Store; PDFs are
 * rendered by a plugin with no DOM to serialize; file:// needs the user to
 * enable "Allow access to file URLs".
 */
export function capturability(
  url: string,
  opts: { mimeIsPdf?: boolean; fileAccess?: boolean } = {},
): Capturability {
  let u: URL;
  try {
    u = new URL(url);
  } catch {
    return { ok: false, reason: "unsupported-scheme" };
  }
  if (BROWSER_SCHEMES.has(u.protocol)) return { ok: false, reason: "browser-page" };
  const isFile = u.protocol === "file:";
  if (!isFile && u.protocol !== "http:" && u.protocol !== "https:") {
    return { ok: false, reason: "unsupported-scheme" };
  }
  if (
    u.hostname === "chromewebstore.google.com" ||
    (u.hostname === "chrome.google.com" &&
      (u.pathname === "/webstore" || u.pathname.startsWith("/webstore/")))
  ) {
    return { ok: false, reason: "web-store" };
  }
  if (isFile && !opts.fileAccess) return { ok: false, reason: "file-access" };
  if (opts.mimeIsPdf || u.pathname.toLowerCase().endsWith(".pdf")) {
    return { ok: false, reason: "pdf" };
  }
  return { ok: true };
}
