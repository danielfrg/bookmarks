import test from "node:test";
import assert from "node:assert/strict";
import { api, ApiError, requestJSON } from "./api.ts";

type Call = { url: string; init: RequestInit };

async function withFetch<T>(
  respond: (url: string, init: RequestInit) => Response,
  fn: (calls: Call[]) => Promise<T>,
): Promise<T> {
  const original = globalThis.fetch;
  const calls: Call[] = [];
  globalThis.fetch = (async (input: RequestInfo | URL, init: RequestInit = {}) => {
    calls.push({ url: String(input), init });
    return respond(String(input), init);
  }) as typeof fetch;
  try {
    return await fn(calls);
  } finally {
    globalThis.fetch = original;
  }
}

const json = (body: unknown, status = 200, type = "application/json") =>
  new Response(JSON.stringify(body), { status, headers: { "Content-Type": type } });

test("requestJSON surfaces problem details", async () => {
  const cases: [Response, string][] = [
    [
      json({ detail: "A bookmark with this URL already exists" }, 409, "application/problem+json"),
      "A bookmark with this URL already exists",
    ],
    [
      json(
        { title: "x", errors: [{ message: "url: is required" }] },
        422,
        "application/problem+json",
      ),
      "url: is required",
    ],
    [
      json(
        { detail: "validation failed", errors: [{ message: "expected string (url)" }] },
        422,
        "application/problem+json",
      ),
      "validation failed: expected string (url)",
    ],
    [json({ detail: "invalid API token" }, 401), "invalid API token"],
    [
      new Response("X-Bookmarks header is required\n", { status: 403 }),
      "X-Bookmarks header is required",
    ],
    [new Response("", { status: 500 }), "Request failed (500). Try again."],
  ];
  for (const [res, message] of cases) {
    await withFetch(
      () => res,
      async () => {
        await assert.rejects(
          () => requestJSON("/api/x"),
          (err: unknown) => {
            assert.ok(err instanceof ApiError);
            assert.equal(err.message, message);
            assert.equal(err.status, res.status);
            return true;
          },
        );
      },
    );
  }
});

test("writes send X-Bookmarks and JSON content type, GETs do not", async () => {
  await withFetch(
    (url) => (url.endsWith("/get") ? json({}) : new Response(null, { status: 204 })),
    async (calls) => {
      await requestJSON("/api/get");
      await requestJSON("/api/x", { method: "POST", body: "{}" });
      await requestJSON("/api/x", { method: "DELETE" });
      const [get, post, del] = calls.map((c) => new Headers(c.init.headers));
      assert.equal(get.get("X-Bookmarks"), null);
      assert.equal(get.get("Content-Type"), null);
      assert.equal(post.get("X-Bookmarks"), "1");
      assert.equal(post.get("Content-Type"), "application/json");
      assert.equal(del.get("X-Bookmarks"), "1");
    },
  );
});

test("204 and empty 202 bodies return undefined", async () => {
  await withFetch(
    () => new Response(null, { status: 204 }),
    async () => assert.equal(await api.remove(1), undefined),
  );
  await withFetch(
    () => new Response("", { status: 202 }),
    async (calls) => {
      assert.equal(await api.capture(3), undefined);
      assert.equal(calls[0].url, "/api/bookmarks/3/capture");
      assert.equal(calls[0].init.method, "POST");
    },
  );
});

test("list builds query string", async () => {
  await withFetch(
    () => json({ count: 0, results: [] }),
    async (calls) => {
      await api.list({ q: "", sort: "", offset: 0 });
      assert.equal(calls[0].url, "/api/bookmarks");
      await api.list({
        q: "a b #go",
        unread: "yes",
        sort: "title_asc",
        savedSearch: 4,
        limit: 30,
        offset: 60,
      });
      const u = new URL(calls[1].url, "http://x");
      assert.equal(u.pathname, "/api/bookmarks");
      assert.equal(u.searchParams.get("q"), "a b #go");
      assert.equal(u.searchParams.get("unread"), "yes");
      assert.equal(u.searchParams.get("saved_search"), "4");
      assert.equal(u.searchParams.get("limit"), "30");
      assert.equal(u.searchParams.get("offset"), "60");
    },
  );
});

test("save reports created from 201 vs 200", async () => {
  for (const [status, created] of [
    [201, true],
    [200, false],
  ] as const) {
    await withFetch(
      () => json({ id: 1 }, status),
      async (calls) => {
        const res = await api.save({ url: "https://a.test" }, "none");
        assert.equal(res.created, created);
        assert.equal(res.bookmark.id, 1);
        assert.equal(calls[0].url, "/api/bookmarks?capture=none");
      },
    );
  }
  await withFetch(
    () => json({ id: 1 }, 201),
    async (calls) => {
      await api.save({ url: "https://a.test" });
      assert.equal(calls[0].url, "/api/bookmarks");
    },
  );
});

test("uploadArchive sends source before file and no content type", async () => {
  await withFetch(
    () => json({ id: 9 }, 201),
    async (calls) => {
      await api.uploadArchive(5, new File(["hi"], "page.html", { type: "text/html" }));
      assert.equal(calls[0].url, "/api/bookmarks/5/archives");
      const headers = new Headers(calls[0].init.headers);
      assert.equal(headers.get("X-Bookmarks"), "1");
      assert.equal(headers.get("Content-Type"), null);
      const form = calls[0].init.body as FormData;
      assert.ok(form instanceof FormData);
      assert.deepEqual([...form.keys()], ["source", "file"]);
      assert.equal(form.get("source"), "upload");
    },
  );
});

test("importNetscape posts a file part without content type", async () => {
  await withFetch(
    () => json({ created: 1, updated: 2, skipped: 3 }),
    async (calls) => {
      const res = await api.importNetscape(new File(["<html>"], "b.html"));
      assert.deepEqual(res, { created: 1, updated: 2, skipped: 3 });
      const headers = new Headers(calls[0].init.headers);
      assert.equal(headers.get("X-Bookmarks"), "1");
      assert.equal(headers.get("Content-Type"), null);
      assert.deepEqual([...(calls[0].init.body as FormData).keys()], ["file"]);
    },
  );
});

test("routes and verbs match the server", async () => {
  await withFetch(
    () => json({}),
    async (calls) => {
      await api.get(2);
      await api.check("https://a.test/?x=1&y=2");
      await api.update(2, { notes: "n" });
      await api.archives(2);
      await api.reader(2, 100, 500);
      await api.reader(2);
      await api.renameTag(7, "new");
      await api.saveSavedSearch({ name: "b" });
      await api.saveSavedSearch({ id: 3, name: "b" });
      await api.reorderSavedSearches([3, 1]);
      await api.putSettings({} as never);
      await api.bulk({ ids: [1], action: "read" });
      await api.extract(2);
      await api.deleteArchive(8);
      await api.deleteTag(7);
      await api.deleteSavedSearch(3);
      const got = calls.map((c) => `${c.init.method ?? "GET"} ${c.url}`);
      assert.deepEqual(got, [
        "GET /api/bookmarks/2",
        "GET /api/bookmarks/check?url=https%3A%2F%2Fa.test%2F%3Fx%3D1%26y%3D2",
        "PATCH /api/bookmarks/2",
        "GET /api/bookmarks/2/archives",
        "GET /api/bookmarks/2/reader?offset=100&max_chars=500",
        "GET /api/bookmarks/2/reader",
        "PATCH /api/tags/7",
        "POST /api/saved-searches",
        "PUT /api/saved-searches/3",
        "POST /api/saved-searches/reorder",
        "PUT /api/settings",
        "POST /api/bookmarks/bulk",
        "POST /api/bookmarks/2/reader/extract",
        "DELETE /api/archives/8",
        "DELETE /api/tags/7",
        "DELETE /api/saved-searches/3",
      ]);
      assert.equal(calls[7].init.body, JSON.stringify({ name: "b" }));
      assert.equal(calls[9].init.body, JSON.stringify({ ids: [3, 1] }));
    },
  );
  assert.equal(api.exportURL, "/api/export/netscape");
});

test("API token client: list, create and delete", async () => {
  const listed = [
    {
      id: 2,
      name: "CLI",
      prefix: "abcdefgh",
      created_at: "2026-11-09T12:00:00Z",
      last_used_at: null,
    },
  ];
  const created = { ...listed[0], id: 3, name: "Ext", token: "bk_abcdefgh123" };
  await withFetch(
    (url, init) =>
      init.method === "POST"
        ? json(created, 201)
        : init.method === "DELETE"
          ? new Response(null, { status: 204 })
          : json(listed),
    async (calls) => {
      assert.deepEqual(await api.tokens(), listed);
      assert.deepEqual(await api.createToken("Ext"), created);
      assert.equal(await api.deleteToken(3), undefined);
      const got = calls.map((c) => `${c.init.method ?? "GET"} ${c.url}`);
      assert.deepEqual(got, ["GET /api/tokens", "POST /api/tokens", "DELETE /api/tokens/3"]);
      assert.equal(calls[1].init.body, JSON.stringify({ name: "Ext" }));
      const post = new Headers(calls[1].init.headers);
      assert.equal(post.get("X-Bookmarks"), "1");
      assert.equal(post.get("Content-Type"), "application/json");
      assert.equal(new Headers(calls[2].init.headers).get("X-Bookmarks"), "1");
    },
  );
});

test("createToken surfaces validation errors", async () => {
  await withFetch(
    () => json({ detail: "name: is required", status: 422 }, 422, "application/problem+json"),
    async () => {
      await assert.rejects(() => api.createToken(""), /name: is required/);
    },
  );
});
