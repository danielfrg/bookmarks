import { test } from "node:test";
import assert from "node:assert/strict";
import { bytesToBase64 } from "./chunks.ts";
import type { RelayFetchRequest, RelayFetchResponse } from "./messages.ts";
import {
  createResourceFetch,
  isPrivateHost,
  relayAllowed,
  relayFetch,
  responseFromRelay,
} from "./relay.ts";

type Call = { url: string; init?: RequestInit };

function fakeFetch(...results: (Response | Error)[]) {
  const calls: Call[] = [];
  const fn = async (url: string, init?: RequestInit) => {
    calls.push({ url, init });
    const r = results.shift();
    if (!r) throw new Error("unexpected fetch");
    if (r instanceof Error) throw r;
    return r;
  };
  return { fn, calls };
}

function fakeRelay(res?: RelayFetchResponse) {
  const calls: RelayFetchRequest[] = [];
  return {
    calls,
    fn: async (req: RelayFetchRequest) => {
      calls.push(req);
      return res;
    },
  };
}

test("page fetch success is returned as-is with SingleFile defaults", async () => {
  const ok = new Response("body", { status: 200 });
  const f = fakeFetch(ok);
  const relay = fakeRelay();
  const fetch = createResourceFetch({ fetch: f.fn, relay: relay.fn });
  const res = await fetch("https://cdn/a.css", { headers: { accept: "text/css" } });
  assert.equal(res, ok);
  assert.deepEqual(f.calls[0]?.init, {
    cache: "force-cache",
    headers: { accept: "text/css" },
    referrerPolicy: "strict-origin-when-cross-origin",
  });
  assert.equal(relay.calls.length, 0);
});

test("401/403/404 retries once without referrer", async () => {
  for (const status of [401, 403, 404]) {
    const second = new Response("ok", { status: 200 });
    const f = fakeFetch(new Response("", { status }), second);
    const fetch = createResourceFetch({ fetch: f.fn, relay: fakeRelay().fn });
    assert.equal(await fetch("https://cdn/a.png"), second);
    assert.equal(f.calls[1]?.init?.referrerPolicy, "no-referrer");
  }
});

test("no retry when SingleFile supplied an explicit referrer", async () => {
  const notFound = new Response("", { status: 404 });
  const f = fakeFetch(notFound);
  const fetch = createResourceFetch({ fetch: f.fn, relay: fakeRelay().fn });
  assert.equal(await fetch("https://cdn/a.png", { referrer: "https://site/" }), notFound);
  assert.equal(f.calls.length, 1);
});

test("CORS failure on both page attempts falls back to the background relay", async () => {
  const f = fakeFetch(new TypeError("Failed to fetch"), new TypeError("Failed to fetch"));
  const relay = fakeRelay({
    ok: true,
    status: 200,
    url: "https://cdn/final.woff2",
    contentType: "font/woff2",
    base64: bytesToBase64(new Uint8Array([1, 2, 3])),
  });
  const fetch = createResourceFetch({ fetch: f.fn, relay: relay.fn });
  const res = await fetch("https://cdn/a.woff2", { referrerPolicy: "origin" });
  assert.deepEqual(relay.calls, [{ type: "relay-fetch", url: "https://cdn/a.woff2" }]);
  assert.equal(res.status, 200);
  assert.equal(res.url, "https://cdn/final.woff2");
  assert.equal(res.headers.get("content-type"), "font/woff2");
  assert.deepEqual(new Uint8Array(await res.arrayBuffer()), new Uint8Array([1, 2, 3]));
});

test("relay errors reject so SingleFile records a failed resource", async () => {
  const fetch = createResourceFetch({
    fetch: fakeFetch(new TypeError("x"), new TypeError("x")).fn,
    relay: fakeRelay({ ok: false, error: "resource too large" }).fn,
  });
  await assert.rejects(fetch("https://cdn/big.png"), /resource too large/);
  const silent = createResourceFetch({
    fetch: fakeFetch(new TypeError("x"), new TypeError("x")).fn,
    relay: fakeRelay(undefined).fn,
  });
  await assert.rejects(silent("https://cdn/big.png"), /no response/);
});

test("relayed response without content type has no content-type header", async () => {
  const res = responseFromRelay({ ok: true, status: 404, url: "u", contentType: "", base64: "" });
  assert.equal(res.headers.get("content-type"), null);
  assert.equal(res.status, 404);
  assert.equal((await res.arrayBuffer()).byteLength, 0);
});

test("background relay fetches with credentials and base64-encodes the body", async () => {
  const body = new Uint8Array([0, 255, 10, 13]);
  const response = new Response(body, { status: 200, headers: { "content-type": "image/png" } });
  const f = fakeFetch(response);
  const res = await relayFetch({ type: "relay-fetch", url: "https://cdn/a.png" }, f.fn, {
    senderURL: "https://news.example/",
  });
  assert.deepEqual(res, {
    ok: true,
    status: 200,
    url: "https://cdn/a.png",
    contentType: "image/png",
    base64: bytesToBase64(body),
  });
  assert.deepEqual(f.calls, [
    { url: "https://cdn/a.png", init: { credentials: "include", redirect: "manual" } },
  ]);
});

test("background relay rejects oversize resources", async () => {
  const declared = new Response("x", { headers: { "content-length": "11" } });
  assert.deepEqual(
    await relayFetch({ type: "relay-fetch", url: "https://a/b" }, fakeFetch(declared).fn, {
      maxBytes: 10,
    }),
    {
      ok: false,
      error: "resource too large",
    },
  );
  const actual = new Response("0123456789A");
  assert.deepEqual(
    await relayFetch({ type: "relay-fetch", url: "https://a/b" }, fakeFetch(actual).fn, {
      maxBytes: 10,
    }),
    {
      ok: false,
      error: "resource too large",
    },
  );
});

test("background relay refuses non-http URLs and reports network errors", async () => {
  const none = fakeFetch();
  assert.equal(
    (await relayFetch({ type: "relay-fetch", url: "file:///etc/passwd" }, none.fn)).ok,
    false,
  );
  assert.equal((await relayFetch({ type: "relay-fetch", url: "nope" }, none.fn)).ok, false);
  assert.equal(none.calls.length, 0);
  assert.deepEqual(
    await relayFetch(
      { type: "relay-fetch", url: "https://a/b" },
      fakeFetch(new TypeError("offline")).fn,
    ),
    { ok: false, error: "offline" },
  );
});

test("network error then 404 retries without referrer only once", async () => {
  const notFound = new Response("", { status: 404 });
  const f = fakeFetch(new TypeError("x"), notFound);
  const fetch = createResourceFetch({ fetch: f.fn, relay: fakeRelay().fn });
  assert.equal(await fetch("https://cdn/a.png"), notFound);
  assert.equal(f.calls.length, 2);
});

const privateHosts = [
  "localhost",
  "LOCALHOST",
  "localhost.",
  "admin.localhost",
  "127.0.0.1",
  "127.1.2.3",
  "0.0.0.0",
  "0.1.2.3",
  "10.0.0.1",
  "172.16.0.1",
  "172.31.255.255",
  "192.168.1.10",
  "169.254.169.254",
  "100.64.0.1",
  "100.127.255.255",
  "::1",
  "[::1]",
  "::",
  "[::ffff:127.0.0.1]",
  "[::ffff:7f00:1]",
  "::ffff:192.168.0.1",
  "[::ffff:c0a8:1]",
  "fc00::1",
  "[fd12:3456::1]",
  "fe80::1",
  "[febf::1]",
  "nas.local",
  "grafana.internal",
  "router.lan",
  "Router.LAN.",
  "pi.home.arpa",
];
const publicHosts = [
  "example.com",
  "cdn.example.com",
  "localhost.example.com",
  "local",
  "8.8.8.8",
  "172.15.0.1",
  "172.32.0.1",
  "100.63.255.255",
  "100.128.0.1",
  "192.169.0.1",
  "11.0.0.1",
  "[2606:4700::1111]",
  "2001:db8::1",
  "[::ffff:8.8.8.8]",
  "fec0::1",
  "lan.example.com",
];
for (const h of privateHosts) {
  test(`isPrivateHost(${h}) is true`, () => assert.equal(isPrivateHost(h), true));
}
for (const h of publicHosts) {
  test(`isPrivateHost(${h}) is false`, () => assert.equal(isPrivateHost(h), false));
}

test("relayAllowed blocks private targets from public pages", () => {
  const cases: [string, string | undefined, boolean][] = [
    ["https://cdn.example.com/a.png", "https://news.example/", true],
    ["https://cdn.example.com/a.png", "http://192.168.1.5/", true],
    ["http://192.168.1.5/admin", "https://news.example/", false],
    ["http://127.0.0.1:8080/x", "https://news.example/", false],
    ["http://2130706433/x", "https://news.example/", false],
    ["http://0x7f.1/x", "https://news.example/", false],
    ["http://[::ffff:127.0.0.1]/x", "https://news.example/", false],
    ["http://nas.local/x", "https://news.example/", false],
    ["http://nas.local/x", undefined, false],
    ["http://nas.local/x", "file:///Users/me/a.html", false],
    ["http://nas.local/x", "http://grafana.internal/d", true],
    ["http://10.0.0.2/x", "http://[::1]:3000/", true],
    ["not a url", "https://news.example/", false],
  ];
  for (const [target, sender, want] of cases) {
    assert.equal(relayAllowed(target, sender), want, `${target} from ${sender}`);
  }
});

function opaqueRedirect(): Response {
  return {
    type: "opaqueredirect",
    status: 0,
    url: "",
    headers: new Headers(),
    body: null,
    arrayBuffer: async () => new ArrayBuffer(0),
  } as unknown as Response;
}

function redirected(body: string, url: string, init?: ResponseInit): Response {
  const r = new Response(body, init);
  Object.defineProperty(r, "url", { value: url });
  return r;
}

const PUBLIC_PAGE = { senderURL: "https://news.example/article" };
const PRIVATE_PAGE = { senderURL: "http://192.168.1.5/dashboard" };

test("public page, redirect to a public host: refetched without cookies", async () => {
  const f = fakeFetch(
    opaqueRedirect(),
    redirected("img", "https://cdn2.example/final.png", {
      headers: { "content-type": "image/png" },
    }),
  );
  const res = await relayFetch(
    { type: "relay-fetch", url: "https://cdn.example/r" },
    f.fn,
    PUBLIC_PAGE,
  );
  assert.deepEqual(f.calls, [
    { url: "https://cdn.example/r", init: { credentials: "include", redirect: "manual" } },
    { url: "https://cdn.example/r", init: { credentials: "omit", redirect: "follow" } },
  ]);
  assert.deepEqual(res, {
    ok: true,
    status: 200,
    url: "https://cdn2.example/final.png",
    contentType: "image/png",
    base64: bytesToBase64(new TextEncoder().encode("img")),
  });
});

test("public page, redirect ending on a private host: refused without the body", async () => {
  for (const final of ["http://192.168.1.5/admin", "http://[::1]/", "http://nas.local/x"]) {
    const f = fakeFetch(opaqueRedirect(), redirected("secret admin page", final));
    const res = await relayFetch(
      { type: "relay-fetch", url: "https://attacker.example/r" },
      f.fn,
      PUBLIC_PAGE,
    );
    assert.deepEqual(res, { ok: false, error: "private host blocked" }, final);
    assert.equal(f.calls[1]?.init?.credentials, "omit");
  }
});

test("public page, private target: refused before fetching", async () => {
  const f = fakeFetch();
  const res = await relayFetch(
    { type: "relay-fetch", url: "http://10.0.0.1/x" },
    f.fn,
    PUBLIC_PAGE,
  );
  assert.deepEqual(res, { ok: false, error: "private host blocked" });
  assert.equal(f.calls.length, 0);
});

test("unknown sender is treated as public", async () => {
  const f = fakeFetch();
  const res = await relayFetch({ type: "relay-fetch", url: "http://127.0.0.1/x" }, f.fn);
  assert.deepEqual(res, { ok: false, error: "private host blocked" });
});

test("private page follows redirects with cookies", async () => {
  const f = fakeFetch(redirected("css", "http://10.0.0.2/app.css"));
  const res = await relayFetch(
    { type: "relay-fetch", url: "http://nas.local/r" },
    f.fn,
    PRIVATE_PAGE,
  );
  assert.deepEqual(f.calls, [
    { url: "http://nas.local/r", init: { credentials: "include", redirect: "follow" } },
  ]);
  assert.equal(res.ok, true);
  assert.equal(res.ok && res.url, "http://10.0.0.2/app.css");
});
