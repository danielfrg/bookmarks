export interface ExtSettings {
  serverURL: string;
  token: string;
  unreadByDefault: boolean;
}

export interface StorageLike {
  get(keys: string[]): Promise<Record<string, unknown>>;
  set(items: Record<string, unknown>): Promise<void>;
}

export const DEFAULTS: ExtSettings = {
  serverURL: "",
  token: "",
  unreadByDefault: false,
};

const KEYS = Object.keys(DEFAULTS);

/** Trims and reduces to scheme://host[:port]; null unless http(s). */
export function normalizeServerURL(raw: string): string | null {
  let url: URL;
  try {
    url = new URL(raw.trim());
  } catch {
    return null;
  }
  if (url.protocol !== "http:" && url.protocol !== "https:") return null;
  if (!url.hostname) return null;
  return url.origin;
}

export function isConfigured(s: ExtSettings): boolean {
  return normalizeServerURL(s.serverURL) !== null && s.token.trim() !== "";
}

function defaultStorage(): StorageLike {
  return chrome.storage.local as unknown as StorageLike;
}

export async function loadSettings(storage: StorageLike = defaultStorage()): Promise<ExtSettings> {
  const raw = await storage.get(KEYS);
  const str = (v: unknown, d: string) => (typeof v === "string" ? v : d);
  const bool = (v: unknown, d: boolean) => (typeof v === "boolean" ? v : d);
  return {
    serverURL: str(raw.serverURL, DEFAULTS.serverURL),
    token: str(raw.token, DEFAULTS.token),
    unreadByDefault: bool(raw.unreadByDefault, DEFAULTS.unreadByDefault),
  };
}

export async function saveSettings(
  s: ExtSettings,
  storage: StorageLike = defaultStorage(),
): Promise<void> {
  await storage.set({ ...s });
}
