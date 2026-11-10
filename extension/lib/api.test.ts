import { test } from "node:test";
import assert from "node:assert/strict";
import {
  ApiError,
  CloudflareLoginError,
  TokenError,
  TooLargeError,
  UnreachableError,
  client,
  isCloudflareLogin,
} from "./api.ts";

const SETTINGS = { serverURL: "https://b.example.com", token: "tok" };

type Call = { url: string; init: RequestInit };

function at(calls: Call[], i: number): Call {
  const call = calls[i];
  assert.ok(call, `expected fetch call ${i}`);
  return call;
}

function stub(respond: (call: Call) => Response | Promise<Response>) {
  const calls: Call[] = [];
  const fetchImpl = (async (input: string | URL | Request, init: RequestInit = {}) => {
    const call = { url: String(input), init };
    calls.push(call);
    return respond(call);
  }) as typeof fetch;
  return { calls, fetchImpl };
}

const json = (body: unknown, init: ResponseInit = {}) =>
  new Response(JSON.stringify(body), {
    headers: { "content-type": "application/json" },
    ...init,
  });

const problem = (status: number, body: unknown) =>
  new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/problem+json" },
  });

/** A Response-like object for shapes the Response constructor cannot build. */
function fake(props: Record<string, unknown>): Response {
  return { ok: true, status: 200, headers: new Headers(), ...props } as unknown as Response;
}

test("detects Cloudflare login responses", async () => {
  assert.equal(
    isCloudflareLogin(
      fake({ redirected: true, url: "https://team.cloudflareaccess.com/cdn-cgi/access/login" }),
    ),
    true,
  );
  assert.equal(isCloudflareLogin(fake({ type: "opaqueredirect", status: 0, ok: false })), true);
  assert.equal(
    isCloudflareLogin(
      fake({ headers: new Headers({ "content-type": "text/html; charset=utf-8" }) }),
    ),
    true,
  );
  assert.equal(
    isCloudflareLogin(
      fake({ status: 403, ok: false, headers: new Headers({ "content-type": "text/html" }) }),
    ),
    true,
  );
  assert.equal(
    isCloudflareLogin(fake({ redirected: true, url: "https://b.example.com/api/tags" })),
    false,
  );
  assert.equal(
    isCloudflareLogin(fake({ headers: new Headers({ "content-type": "application/json" }) })),
    false,
  );

  const { fetchImpl } = stub(
    () => new Response("<html></html>", { headers: { "content-type": "text/html" } }),
  );
  await assert.rejects(client(SETTINGS, fetchImpl).ping(), CloudflareLoginError);
});

test("401 maps to TokenError", async () => {
  const { fetchImpl } = stub(() =>
    problem(401, { title: "Unauthorized", detail: "invalid token" }),
  );
  await assert.rejects(client(SETTINGS, fetchImpl).tags(), (err: unknown) => {
    assert.ok(err instanceof TokenError);
    assert.ok(err instanceof ApiError);
    assert.equal(err.status, 401);
    assert.equal(err.message, "invalid token");
    return true;
  });
});

test("413 maps to a size error", async () => {
  const { fetchImpl } = stub(() =>
    problem(413, { detail: "archive exceeds the upload limit of 50 MB" }),
  );
  await assert.rejects(
    client(SETTINGS, fetchImpl).uploadArchive(1, new Blob(["x"]), "a.html"),
    (err: unknown) => {
      assert.ok(err instanceof TooLargeError);
      assert.equal(err.status, 413);
      return true;
    },
  );
  const noHint = stub(() => new Response("too big", { status: 413 }));
  await assert.rejects(
    client(SETTINGS, noHint.fetchImpl).uploadArchive(1, new Blob(["x"]), "a.html"),
    (err: unknown) => err instanceof TooLargeError && err.limitMB === undefined,
  );
});

test("parses a limit hint from a 413 message", async () => {
  const { fetchImpl } = stub(
    () => new Response("request body too large: limit 25 MB", { status: 413 }),
  );
  await assert.rejects(
    client(SETTINGS, fetchImpl).uploadArchive(1, new Blob(["x"]), "a.html"),
    (err: unknown) => err instanceof TooLargeError && err.limitMB === 25,
  );
});

test("network failure maps to UnreachableError", async () => {
  const fetchImpl = (async () => {
    throw new TypeError("Failed to fetch");
  }) as typeof fetch;
  await assert.rejects(client(SETTINGS, fetchImpl).ping(), UnreachableError);
});

test("error messages come from problem+json, text, or the status", async () => {
  const both = stub(() =>
    problem(422, { detail: "validation failed", errors: [{ message: "url is invalid" }] }),
  );
  await assert.rejects(client(SETTINGS, both.fetchImpl).check("x"), (e: unknown) => {
    assert.ok(e instanceof ApiError && !(e instanceof TokenError));
    assert.equal((e as ApiError).status, 422);
    assert.equal((e as Error).message, "validation failed: url is invalid");
    return true;
  });
  const text = stub(
    () => new Response("  boom \n", { status: 500, headers: { "content-type": "text/plain" } }),
  );
  await assert.rejects(client(SETTINGS, text.fetchImpl).check("x"), /^ApiError: boom$/);
  const empty = stub(() => new Response("", { status: 502 }));
  await assert.rejects(client(SETTINGS, empty.fetchImpl).check("x"), /Request failed \(502\)/);
});

test("save sends capture param and bearer, reports created from 201", async () => {
  const bookmark = { id: 7, url: "https://e.com", title: "E" };
  const { calls, fetchImpl } = stub(({ init }) => {
    void init;
    return json(bookmark, { status: 201 });
  });
  const c = client(SETTINGS, fetchImpl);
  const res = await c.save({ url: "https://e.com", tags: ["a"] }, "none");
  assert.equal(res.created, true);
  assert.equal(res.bookmark.id, 7);
  assert.equal(at(calls, 0).url, "https://b.example.com/api/bookmarks?capture=none");
  const headers = new Headers(at(calls, 0).init.headers);
  assert.equal(headers.get("authorization"), "Bearer tok");
  assert.equal(headers.get("content-type"), "application/json");
  assert.equal(headers.has("x-bookmarks"), false);
  assert.equal(at(calls, 0).init.method, "POST");
  assert.equal(at(calls, 0).init.redirect, "follow");
  assert.deepEqual(JSON.parse(at(calls, 0).init.body as string), {
    url: "https://e.com",
    tags: ["a"],
  });

  const again = stub(() => json(bookmark, { status: 200 }));
  const res2 = await client(SETTINGS, again.fetchImpl).save({ url: "https://e.com" }, "server");
  assert.equal(res2.created, false);
  assert.equal(at(again.calls, 0).url, "https://b.example.com/api/bookmarks?capture=server");
});

test("uploadArchive puts source before file", async () => {
  const { calls, fetchImpl } = stub(() => json({ id: 1 }, { status: 201 }));
  await client(SETTINGS, fetchImpl).uploadArchive(42, new Blob(["<html></html>"]), "page.html");
  assert.equal(at(calls, 0).url, "https://b.example.com/api/bookmarks/42/archives");
  assert.equal(at(calls, 0).init.method, "POST");
  const form = at(calls, 0).init.body as FormData;
  assert.ok(form instanceof FormData);
  const entries = [...form.entries()];
  assert.deepEqual(
    entries.map(([k]) => k),
    ["source", "file"],
  );
  assert.equal(entries[0]![1], "extension");
  assert.equal((entries[1]![1] as File).name, "page.html");
  const headers = new Headers(at(calls, 0).init.headers);
  assert.equal(headers.get("authorization"), "Bearer tok");
  assert.equal(headers.has("content-type"), false);
});

test("check url-encodes", async () => {
  const result = { bookmark: null, basic_capture_only: false, metadata: {}, auto_tags: [] };
  const { calls, fetchImpl } = stub(() => json(result));
  const res = await client(SETTINGS, fetchImpl).check("https://e.com/a?b=1&c=d e#frag");
  assert.equal(res.bookmark, null);
  assert.equal(
    at(calls, 0).url,
    "https://b.example.com/api/bookmarks/check?url=" +
      encodeURIComponent("https://e.com/a?b=1&c=d e#frag"),
  );
  assert.equal(at(calls, 0).init.method ?? "GET", "GET");
});

test("tags returns names and ping hits /api/tags", async () => {
  const { calls, fetchImpl } = stub(() =>
    json([
      { id: 1, name: "go", count: 2 },
      { id: 2, name: "svelte", count: 1 },
    ]),
  );
  const c = client({ serverURL: "https://b.example.com/", token: "tok" }, fetchImpl);
  assert.deepEqual(await c.tags(), ["go", "svelte"]);
  await c.ping();
  assert.equal(at(calls, 1).url, "https://b.example.com/api/tags");
});

test("an HTML error page from a proxy yields a generic message", async () => {
  const page = () =>
    new Response("<html><body>Bad Gateway</body></html>", {
      status: 502,
      headers: { "content-type": "text/html; charset=utf-8" },
    });
  await assert.rejects(client(SETTINGS, stub(page).fetchImpl).ping(), (err: unknown) => {
    assert.ok(err instanceof ApiError);
    assert.equal(err.status, 502);
    assert.equal(err.message, "Request failed (502)");
    return true;
  });
});
