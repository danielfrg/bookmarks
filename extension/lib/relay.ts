import { base64ToBytes, bytesToBase64 } from "./chunks.ts";
import { RELAY_MAX_BYTES, type RelayFetchRequest, type RelayFetchResponse } from "./messages.ts";

type FetchFn = (input: string, init?: RequestInit) => Promise<Response>;

/**
 * What single-file-core's `init({ fetch, frameFetch })` expects a fetch to
 * resolve to (core/util.js fetchContent): `status`, `headers.get()`,
 * `arrayBuffer()` and optionally the final `url`. A real Response qualifies;
 * so does the object SingleFile's own extension builds for relayed responses.
 */
export interface ResourceResponse {
  status: number;
  url?: string;
  headers: { get(name: string): string | null };
  arrayBuffer(): Promise<ArrayBuffer>;
}

/** Options single-file-core passes to the fetch function. */
export interface ResourceFetchOptions {
  cache?: RequestCache;
  referrer?: string;
  referrerPolicy?: ReferrerPolicy;
  headers?: Record<string, string>;
  frameId?: string;
}

const RETRY_WITHOUT_REFERRER = new Set([401, 403, 404]);

/**
 * The fetch function handed to single-file-core, mirroring SingleFile's
 * content-fetch.js: fetch from the page first (same CORS rules as the page),
 * retry once with no referrer on 401/403/404 or a network error, and when the
 * page cannot read the resource at all (CORS, mixed content) ask the
 * background to fetch it with the extension's host permissions.
 */
export function createResourceFetch(deps: {
  fetch: FetchFn;
  relay: (req: RelayFetchRequest) => Promise<RelayFetchResponse | undefined>;
}) {
  return async function resourceFetch(
    url: string,
    options: ResourceFetchOptions = {},
  ): Promise<ResourceResponse> {
    const base: RequestInit = {
      cache: options.cache || "force-cache",
      headers: options.headers,
      referrerPolicy: options.referrerPolicy || "strict-origin-when-cross-origin",
    };
    const canDropReferrer = base.referrerPolicy !== "no-referrer" && !options.referrer;
    const noReferrer = (): Promise<Response> =>
      deps.fetch(url, { ...base, referrerPolicy: "no-referrer" });
    try {
      let response: Response;
      let retried = false;
      try {
        response = await deps.fetch(url, base);
      } catch (error) {
        if (!canDropReferrer) throw error;
        retried = true;
        response = await noReferrer();
      }
      if (RETRY_WITHOUT_REFERRER.has(response.status) && canDropReferrer && !retried) {
        response = await noReferrer();
      }
      return response;
    } catch {
      const res = await deps.relay({ type: "relay-fetch", url });
      if (!res) throw new Error("relay fetch: no response from background");
      if (!res.ok) throw new Error(`relay fetch: ${res.error}`);
      return responseFromRelay(res);
    }
  };
}

export function responseFromRelay(
  res: Extract<RelayFetchResponse, { ok: true }>,
): ResourceResponse {
  const headers = new Headers();
  if (res.contentType) headers.set("content-type", res.contentType);
  return {
    status: res.status,
    url: res.url,
    headers,
    arrayBuffer: async () => {
      const bytes = base64ToBytes(res.base64);
      return bytes.buffer.slice(
        bytes.byteOffset,
        bytes.byteOffset + bytes.byteLength,
      ) as ArrayBuffer;
    },
  };
}

const PRIVATE_HOST_BLOCKED: RelayFetchResponse = { ok: false, error: "private host blocked" };

/**
 * Background side of the relay: fetch with the extension's privileges and
 * base64 the body.
 *
 * Credentialed relay fetches bypass Chrome's Local Network Access checks, so
 * for a page on a public host the relay never returns a private host's body:
 * private targets are refused up front, and redirects (whose Location a
 * service worker cannot read with `redirect: "manual"`) are re-fetched
 * without cookies and refused if they end on a private host. Pages that are
 * themselves on a private host keep cookies and plain redirect following.
 */
export async function relayFetch(
  req: RelayFetchRequest,
  fetchFn: FetchFn,
  opts: { senderURL?: string; maxBytes?: number } = {},
): Promise<RelayFetchResponse> {
  const maxBytes = opts.maxBytes ?? RELAY_MAX_BYTES;
  let protocol: string;
  try {
    protocol = new URL(req.url).protocol;
  } catch {
    return { ok: false, error: "invalid URL" };
  }
  if (protocol !== "http:" && protocol !== "https:") {
    return { ok: false, error: `unsupported scheme ${protocol}` };
  }
  if (!relayAllowed(req.url, opts.senderURL)) return PRIVATE_HOST_BLOCKED;
  const privateSender = senderIsPrivate(opts.senderURL);
  try {
    // No `referrer`: a service worker silently replaces a cross-origin one.
    let response = await fetchFn(req.url, {
      credentials: "include",
      redirect: privateSender ? "follow" : "manual",
    });
    if (response.type === "opaqueredirect") {
      response = await fetchFn(req.url, { credentials: "omit", redirect: "follow" });
      if (!relayAllowed(response.url || req.url, opts.senderURL)) {
        await response.body?.cancel().catch(() => {});
        return PRIVATE_HOST_BLOCKED;
      }
    }
    const declared = Number(response.headers.get("content-length"));
    if (declared > maxBytes) {
      await response.body?.cancel().catch(() => {});
      return { ok: false, error: "resource too large" };
    }
    const bytes = new Uint8Array(await response.arrayBuffer());
    if (bytes.byteLength > maxBytes) return { ok: false, error: "resource too large" };
    return {
      ok: true,
      status: response.status,
      url: response.url || req.url,
      contentType: response.headers.get("content-type") ?? "",
      base64: bytesToBase64(bytes),
    };
  } catch (error) {
    return { ok: false, error: error instanceof Error ? error.message : String(error) };
  }
}

function senderIsPrivate(senderURL: string | undefined): boolean {
  if (!senderURL) return false;
  try {
    return isPrivateHost(new URL(senderURL).hostname);
  } catch {
    return false;
  }
}

const PRIVATE_SUFFIXES = [".localhost", ".local", ".internal", ".lan", ".home.arpa"];

/**
 * Whether `hostname` (as in `URL.hostname`; IPv6 may be bracketed) names
 * loopback, private, CGNAT, link-local or unspecified addresses, or a
 * local-only DNS name. The relay fetches with the user's cookies and without
 * Chrome's Local Network Access checks, so it must not let a public page
 * reach these hosts.
 */
export function isPrivateHost(hostname: string): boolean {
  let h = hostname.toLowerCase();
  if (h.startsWith("[") && h.endsWith("]")) h = h.slice(1, -1);
  if (h.endsWith(".")) h = h.slice(0, -1);
  if (h === "localhost" || PRIVATE_SUFFIXES.some((s) => h.endsWith(s))) return true;
  const v4 = parseIPv4(h);
  if (v4) return isPrivateIPv4(v4);
  const v6 = parseIPv6(h);
  if (v6) return isPrivateIPv6(v6);
  return false;
}

/**
 * Whether the background may relay a fetch of `targetURL` for a capture of
 * the page at `senderURL`: private hosts only for pages that are themselves
 * on a private host.
 */
export function relayAllowed(targetURL: string, senderURL: string | undefined): boolean {
  let target: URL;
  try {
    target = new URL(targetURL);
  } catch {
    return false;
  }
  return !isPrivateHost(target.hostname) || senderIsPrivate(senderURL);
}

function parseIPv4(h: string): number[] | null {
  const parts = h.split(".");
  if (parts.length !== 4 || !parts.every((p) => /^\d{1,3}$/.test(p))) return null;
  const n = parts.map(Number);
  return n.every((x) => x <= 255) ? n : null;
}

function isPrivateIPv4([a, b]: number[]): boolean {
  if (a === undefined || b === undefined) return false;
  return (
    a === 0 || // 0.0.0.0/8 ("this host")
    a === 127 ||
    a === 10 ||
    (a === 172 && b >= 16 && b <= 31) ||
    (a === 192 && b === 168) ||
    (a === 169 && b === 254) ||
    (a === 100 && b >= 64 && b <= 127)
  );
}

/** Eight 16-bit groups, or null. Accepts "::" compression and a dotted IPv4 tail. */
function parseIPv6(h: string): number[] | null {
  if (!h.includes(":") || h.includes("%")) return null;
  const lastColon = h.lastIndexOf(":");
  const last = h.slice(lastColon + 1);
  if (last.includes(".")) {
    // Rewrite a dotted IPv4 tail (::ffff:1.2.3.4) as two hex groups.
    const v4 = parseIPv4(last);
    if (!v4) return null;
    const hex = (a: number, b: number) => ((a << 8) | b).toString(16);
    h = h.slice(0, lastColon + 1) + hex(v4[0]!, v4[1]!) + ":" + hex(v4[2]!, v4[3]!);
  }
  const halves = h.split("::");
  if (halves.length > 2) return null;
  const group = (s: string) => (s === "" ? [] : s.split(":"));
  const head = group(halves[0]!);
  const rest = halves.length === 2 ? group(halves[1]!) : [];
  if (![...head, ...rest].every((g) => /^[0-9a-f]{1,4}$/.test(g))) return null;
  const known = head.length + rest.length;
  if (halves.length === 1 ? known !== 8 : known > 7) return null;
  const zeros: number[] = Array.from({ length: 8 - known }, () => 0);
  return [...head.map((g) => parseInt(g, 16)), ...zeros, ...rest.map((g) => parseInt(g, 16))];
}

function isPrivateIPv6(g: number[]): boolean {
  const first = g[0]!;
  const allZeroBefore = (n: number) => g.slice(0, n).every((x) => x === 0);
  if (allZeroBefore(7) && (g[7] === 0 || g[7] === 1)) return true; // :: and ::1
  if (allZeroBefore(5) && g[5] === 0xffff) {
    // IPv4-mapped ::ffff:a.b.c.d
    return isPrivateIPv4([g[6]! >> 8, g[6]! & 0xff, g[7]! >> 8, g[7]! & 0xff]);
  }
  return (first & 0xfe00) === 0xfc00 || (first & 0xffc0) === 0xfe80;
}
