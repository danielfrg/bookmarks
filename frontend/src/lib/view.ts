import type { ListParams } from "./api.ts";

export type Filter = "active" | "unread" | "archived";
export type PageName = "list" | "tags" | "saved" | "settings";
export interface View {
  q: string;
  filter: Filter;
  sort: string;
  saved: number | null;
  page: number;
  view: number | null;
  edit: number | null;
  create: { url: string; title: string } | null;
  read: number | null;
  pageName: PageName;
}

const DEFAULT_SORT = "added_desc";
const SORTS = ["added_desc", "added_asc", "title_asc", "title_desc"];
const FILTERS: Filter[] = ["active", "unread", "archived"];
const PAGES: PageName[] = ["tags", "saved", "settings"];

function positiveInt(value: string | null): number | null {
  if (value === null || !/^\d+$/.test(value)) return null;
  const n = Number(value);
  return Number.isSafeInteger(n) && n >= 1 ? n : null;
}

export function readView(url: URL): View {
  const p = url.searchParams;
  const filter = p.get("filter") as Filter;
  const sort = p.get("sort") ?? "";
  const pageName = p.get("page_name") as PageName;
  return {
    q: p.get("q") ?? "",
    filter: FILTERS.includes(filter) ? filter : "active",
    sort: SORTS.includes(sort) ? sort : DEFAULT_SORT,
    saved: positiveInt(p.get("saved")),
    page: positiveInt(p.get("page")) ?? 1,
    view: positiveInt(p.get("view")),
    edit: positiveInt(p.get("edit")),
    create: p.has("new") ? { url: p.get("url") ?? "", title: p.get("title") ?? "" } : null,
    read: positiveInt(p.get("read")),
    pageName: PAGES.includes(pageName) ? pageName : "list",
  };
}

/** "?..." for the view with defaults omitted, "" when everything is default. */
export function writeView(v: View): string {
  const p = new URLSearchParams();
  if (v.q) p.set("q", v.q);
  if (v.filter !== "active") p.set("filter", v.filter);
  if (v.sort !== DEFAULT_SORT) p.set("sort", v.sort);
  if (v.saved !== null) p.set("saved", String(v.saved));
  if (v.page > 1) p.set("page", String(v.page));
  if (v.view !== null) p.set("view", String(v.view));
  if (v.edit !== null) p.set("edit", String(v.edit));
  if (v.create) {
    p.set("new", "1");
    if (v.create.url) p.set("url", v.create.url);
    if (v.create.title) p.set("title", v.create.title);
  }
  if (v.read !== null) p.set("read", String(v.read));
  if (v.pageName !== "list") p.set("page_name", v.pageName);
  const s = p.toString();
  return s ? `?${s}` : "";
}

/**
 * The path+search to move to: the current view with `patch` applied. Changing
 * the search, filter, sort or saved search without naming a page goes back to page 1.
 */
export function nextURL(current: URL, patch: Partial<View>): string {
  const v = { ...readView(current), ...patch };
  if (patch.page === undefined) {
    const old = readView(current);
    if (v.q !== old.q || v.filter !== old.filter || v.sort !== old.sort || v.saved !== old.saved) {
      v.page = 1;
    }
  }
  return current.pathname + writeView(v);
}

/** Pushes (or replaces) the next view in history and tells listeners. */
export function navigate(patch: Partial<View>, opts: { replace?: boolean } = {}): void {
  const target = nextURL(new URL(window.location.href), patch);
  if (opts.replace) window.history.replaceState(null, "", target);
  else window.history.pushState(null, "", target);
  window.dispatchEvent(new Event("viewchange"));
}

export function listParams(v: View, pageSize: number): ListParams {
  const params: ListParams = {
    q: v.q,
    scope: v.filter === "archived" ? "archived" : "active",
    sort: v.sort,
    limit: pageSize,
    offset: (v.page - 1) * pageSize,
  };
  if (v.filter === "unread") params.unread = "yes";
  if (v.saved !== null) params.savedSearch = v.saved;
  return params;
}
