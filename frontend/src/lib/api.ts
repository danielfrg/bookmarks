export interface Bookmark {
  id: number;
  url: string;
  title: string;
  description: string;
  notes: string;
  tags: string[];
  unread: boolean;
  is_archived: boolean;
  favicon_url: string;
  preview_image_url: string;
  web_archive_url: string;
  date_added: string;
  date_modified: string;
  archive_count: number;
  basic_capture_only: boolean;
  reader_status: "ok" | "failed" | "none";
  /** Only on list results; "" unless the search had a query. */
  snippet?: string;
}
export interface BookmarkList {
  count: number;
  results: Bookmark[];
}
export interface Archive {
  id: number;
  source: "extension" | "server" | "upload" | "import";
  content_type: string;
  display_name: string;
  file_size: number;
  created_at: string;
  view_url: string;
  download_url: string;
}
export interface ReaderPage {
  status: "ok" | "failed" | "none";
  error: string;
  markdown: string;
  offset: number;
  total_chars: number;
  next_offset: number | null;
  word_count: number;
  extracted_at: string | null;
}
export interface CheckResult {
  bookmark: Bookmark | null;
  basic_capture_only: boolean;
  metadata: { title: string; description: string; preview_image_url: string };
  auto_tags: string[];
}
export interface Tag {
  id: number;
  name: string;
  count: number;
}
export interface SavedSearch {
  id: number;
  name: string;
  query: string;
  any_tags: string[];
  all_tags: string[];
  excluded_tags: string[];
  filter_unread: "off" | "yes" | "no";
  order: number;
  date_created: string;
  date_modified: string;
}
export interface Settings {
  favicons_enabled: boolean;
  previews_enabled: boolean;
  wayback_enabled: boolean;
  default_unread: boolean;
  auto_tag_rules: string;
  ui: { items_per_page?: number } & Record<string, unknown>;
}
export interface SaveInput {
  url: string;
  title?: string;
  description?: string;
  notes?: string;
  tags?: string[];
  unread?: boolean;
  is_archived?: boolean;
}
export type BulkAction =
  | "archive"
  | "unarchive"
  | "delete"
  | "tag"
  | "untag"
  | "read"
  | "unread"
  | "refresh_metadata"
  | "capture"
  | "extract";
export interface ListParams {
  q?: string;
  scope?: "active" | "archived" | "all";
  unread?: "yes" | "no";
  sort?: string;
  savedSearch?: number;
  limit?: number;
  offset?: number;
}
export interface BulkRequest {
  ids?: number[];
  query?: { q?: string; scope?: string; unread?: string };
  action: BulkAction;
  tags?: string[];
}
/** An API token as listed; the secret itself is never returned again. */
export interface ApiToken {
  id: number;
  name: string;
  /** First characters after `bk_`, for recognizing the token. */
  prefix: string;
  created_at: string;
  last_used_at: string | null;
}
/** The create response: the only time the full token is available. */
export interface CreatedApiToken extends ApiToken {
  token: string;
}
export interface ImportResult {
  created: number;
  updated: number;
  skipped: number;
}

export class ApiError extends Error {
  status: number;
  constructor(message: string, status: number) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
}

async function send(path: string, init: RequestInit = {}): Promise<Response> {
  const headers = new Headers(init.headers);
  const method = (init.method ?? "GET").toUpperCase();
  if (method !== "GET" && method !== "HEAD") {
    headers.set("X-Bookmarks", "1");
    // Multipart bodies need the browser-generated boundary: leave the type unset.
    if (!(init.body instanceof FormData)) headers.set("Content-Type", "application/json");
  }
  const response = await fetch(path, { ...init, headers });
  if (!response.ok) {
    let message = `Request failed (${response.status}). Try again.`;
    const text = await response.text();
    try {
      const error = JSON.parse(text) as { detail?: string; errors?: { message?: string }[] };
      const first = error.errors?.[0]?.message;
      message =
        error.detail && first ? `${error.detail}: ${first}` : error.detail || first || message;
    } catch {
      if (text.trim()) message = text.trim();
    }
    throw new ApiError(message, response.status);
  }
  return response;
}

export async function requestJSON<T>(path: string, init: RequestInit = {}): Promise<T> {
  const response = await send(path, init);
  // 204 and the bodyless 202s (capture, extract) carry nothing to parse.
  const text = await response.text();
  return (text ? JSON.parse(text) : undefined) as T;
}

function qs(params: Record<string, string | number | undefined>): string {
  const sp = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    if (value === undefined || value === "" || value === 0) continue;
    sp.set(key, String(value));
  }
  const s = sp.toString();
  return s ? `?${s}` : "";
}

const base = "/api";
const body = (value: unknown) => JSON.stringify(value);

export const api = {
  list: (p: ListParams, signal?: AbortSignal) =>
    requestJSON<BookmarkList>(
      `${base}/bookmarks${qs({
        q: p.q,
        scope: p.scope,
        unread: p.unread,
        sort: p.sort,
        saved_search: p.savedSearch,
        limit: p.limit,
        offset: p.offset,
      })}`,
      { signal },
    ),
  get: (id: number, signal?: AbortSignal) =>
    requestJSON<Bookmark>(`${base}/bookmarks/${id}`, { signal }),
  check: (url: string, signal?: AbortSignal) =>
    requestJSON<CheckResult>(`${base}/bookmarks/check${qs({ url })}`, { signal }),
  async save(
    input: SaveInput,
    capture?: "server" | "none",
  ): Promise<{ bookmark: Bookmark; created: boolean }> {
    const response = await send(`${base}/bookmarks${qs({ capture })}`, {
      method: "POST",
      body: body(input),
    });
    return { bookmark: (await response.json()) as Bookmark, created: response.status === 201 };
  },
  update: (id: number, patch: Partial<SaveInput>) =>
    requestJSON<Bookmark>(`${base}/bookmarks/${id}`, { method: "PATCH", body: body(patch) }),
  remove: (id: number) => requestJSON<void>(`${base}/bookmarks/${id}`, { method: "DELETE" }),
  archives: (id: number) => requestJSON<Archive[]>(`${base}/bookmarks/${id}/archives`),
  uploadArchive(id: number, file: File, source: "upload" = "upload") {
    // The server reads parts in order: source must precede file.
    const form = new FormData();
    form.append("source", source);
    form.append("file", file);
    return requestJSON<Archive>(`${base}/bookmarks/${id}/archives`, { method: "POST", body: form });
  },
  deleteArchive: (archiveID: number) =>
    requestJSON<void>(`${base}/archives/${archiveID}`, { method: "DELETE" }),
  capture: (id: number) => requestJSON<void>(`${base}/bookmarks/${id}/capture`, { method: "POST" }),
  extract: (id: number) =>
    requestJSON<void>(`${base}/bookmarks/${id}/reader/extract`, { method: "POST" }),
  reader: (id: number, offset?: number, maxChars?: number) =>
    requestJSON<ReaderPage>(`${base}/bookmarks/${id}/reader${qs({ offset, max_chars: maxChars })}`),
  tags: () => requestJSON<Tag[]>(`${base}/tags`),
  renameTag: (id: number, name: string) =>
    requestJSON<Tag>(`${base}/tags/${id}`, { method: "PATCH", body: body({ name }) }),
  deleteTag: (id: number) => requestJSON<void>(`${base}/tags/${id}`, { method: "DELETE" }),
  savedSearches: () => requestJSON<SavedSearch[]>(`${base}/saved-searches`),
  saveSavedSearch(b: Partial<SavedSearch> & { name: string }) {
    const { id, name, query, any_tags, all_tags, excluded_tags, filter_unread } = b;
    const payload = { name, query, any_tags, all_tags, excluded_tags, filter_unread };
    return requestJSON<SavedSearch>(
      id ? `${base}/saved-searches/${id}` : `${base}/saved-searches`,
      {
        method: id ? "PUT" : "POST",
        body: body(payload),
      },
    );
  },
  deleteSavedSearch: (id: number) =>
    requestJSON<void>(`${base}/saved-searches/${id}`, { method: "DELETE" }),
  reorderSavedSearches: (ids: number[]) =>
    requestJSON<void>(`${base}/saved-searches/reorder`, { method: "POST", body: body({ ids }) }),
  settings: () => requestJSON<Settings>(`${base}/settings`),
  putSettings: (s: Settings) =>
    requestJSON<Settings>(`${base}/settings`, { method: "PUT", body: body(s) }),
  bulk: (req: BulkRequest) =>
    requestJSON<{ affected: number }>(`${base}/bookmarks/bulk`, {
      method: "POST",
      body: body(req),
    }),
  importNetscape(file: File) {
    const form = new FormData();
    form.append("file", file);
    return requestJSON<ImportResult>(`${base}/import/netscape`, { method: "POST", body: form });
  },
  exportURL: `${base}/export/netscape`,
  tokens: () => requestJSON<ApiToken[]>(`${base}/tokens`),
  createToken: (name: string) =>
    requestJSON<CreatedApiToken>(`${base}/tokens`, { method: "POST", body: body({ name }) }),
  deleteToken: (id: number) => requestJSON<void>(`${base}/tokens/${id}`, { method: "DELETE" }),
};
