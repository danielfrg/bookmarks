import { normalizeServerURL } from "./settings.ts";

export interface Bookmark {
  id: number;
  url: string;
  title: string;
  description: string;
  notes: string;
  tags: string[];
  unread: boolean;
  date_added: string;
  basic_capture_only: boolean;
  archive_count: number;
}

export interface CheckResult {
  bookmark: Bookmark | null;
  basic_capture_only: boolean;
  metadata: { title: string; description: string; preview_image_url: string };
  auto_tags: string[];
}

export interface SaveInput {
  url: string;
  title?: string;
  description?: string;
  notes?: string;
  tags?: string[];
  unread?: boolean;
}

export class ApiError extends Error {
  status: number;
  constructor(message: string, status: number) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
}

/** 401: the token is missing, wrong or revoked. */
export class TokenError extends ApiError {
  constructor(message: string, status = 401) {
    super(message, status);
    this.name = "TokenError";
  }
}

/** The request hit a Cloudflare Access login page instead of the API. */
export class CloudflareLoginError extends ApiError {
  constructor(message: string, status: number) {
    super(message, status);
    this.name = "CloudflareLoginError";
  }
}

/** Network failure, DNS error or connection refused. */
export class UnreachableError extends Error {
  constructor(message = "Could not reach the server") {
    super(message);
    this.name = "UnreachableError";
  }
}

/** 413: the archive exceeds the server's upload limit. */
export class TooLargeError extends ApiError {
  limitMB?: number;
  constructor(message: string, limitMB?: number) {
    super(message, 413);
    this.name = "TooLargeError";
    this.limitMB = limitMB;
  }
}

/** True when a response is a Cloudflare Access login instead of an API answer. */
export function isCloudflareLogin(res: Response): boolean {
  if (res.type === "opaqueredirect") return true;
  if (res.redirected) {
    try {
      if (new URL(res.url).hostname.endsWith(".cloudflareaccess.com")) return true;
    } catch {
      /* Unparseable URL: fall through to the content-type check. */
    }
  }
  const htmlOnJSON =
    res.status < 500 && /^text\/html\b/i.test(res.headers.get("content-type") ?? "");
  return htmlOnJSON;
}

async function errorMessage(res: Response): Promise<string> {
  const fallback = `Request failed (${res.status})`;
  // A proxy's HTML error page is not a useful message.
  if (/^text\/html\b/i.test(res.headers.get("content-type") ?? "")) return fallback;
  let text = "";
  try {
    text = await res.text();
  } catch {
    return fallback;
  }
  try {
    const body = JSON.parse(text) as { detail?: string; errors?: { message?: string }[] };
    const first = body.errors?.[0]?.message;
    const detail = body.detail;
    if (detail && first) return `${detail}: ${first}`;
    if (detail || first) return (detail || first) as string;
  } catch {
    /* Not JSON: use the text below. */
  }
  return text.trim() || fallback;
}

export function client(s: { serverURL: string; token: string }, fetchImpl: typeof fetch = fetch) {
  const base = normalizeServerURL(s.serverURL) ?? s.serverURL.trim().replace(/\/+$/, "");

  async function send(path: string, init: RequestInit = {}): Promise<Response> {
    const headers = new Headers(init.headers);
    headers.set("Authorization", `Bearer ${s.token}`);
    const isJSONBody = typeof init.body === "string";
    if (isJSONBody) headers.set("Content-Type", "application/json");
    let res: Response;
    try {
      res = await fetchImpl(`${base}${path}`, { ...init, headers, redirect: "follow" });
    } catch (err) {
      if (err instanceof TypeError) throw new UnreachableError(err.message);
      throw err;
    }
    if (res.type === "opaqueredirect" || (res.redirected && isCloudflareLogin(res))) {
      throw new CloudflareLoginError("Cloudflare Access login required", res.status);
    }
    if (res.status === 413) {
      const message = await errorMessage(res);
      const hint = /limit[^\d]{0,20}(\d+(?:\.\d+)?)\s*MB/i.exec(message);
      throw new TooLargeError(message, hint ? Number(hint[1]) : undefined);
    }
    if (isCloudflareLogin(res)) {
      throw new CloudflareLoginError("Cloudflare Access login required", res.status);
    }
    if (!res.ok) {
      const message = await errorMessage(res);
      if (res.status === 401) throw new TokenError(message);
      throw new ApiError(message, res.status);
    }
    return res;
  }

  return {
    async check(url: string): Promise<CheckResult> {
      const res = await send(`/api/bookmarks/check?url=${encodeURIComponent(url)}`);
      return (await res.json()) as CheckResult;
    },

    async tags(): Promise<string[]> {
      const res = await send("/api/tags");
      return ((await res.json()) as { name: string }[]).map((t) => t.name);
    },

    async save(
      input: SaveInput,
      capture: "none" | "server",
    ): Promise<{ bookmark: Bookmark; created: boolean }> {
      const res = await send(`/api/bookmarks?capture=${capture}`, {
        method: "POST",
        body: JSON.stringify(input),
      });
      return { bookmark: (await res.json()) as Bookmark, created: res.status === 201 };
    },

    async uploadArchive(id: number, html: Blob, filename: string): Promise<void> {
      const form = new FormData();
      // The server reads fields in order: `source` must come before `file`.
      form.append("source", "extension");
      form.append("file", html, filename);
      await send(`/api/bookmarks/${id}/archives`, { method: "POST", body: form });
    },

    async ping(): Promise<void> {
      await send("/api/tags");
    },
  };
}
