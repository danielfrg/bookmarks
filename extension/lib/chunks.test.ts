import { test } from "node:test";
import assert from "node:assert/strict";
import { CHUNK_CHARS, base64ToBytes, bytesToBase64, joinChunks, splitString } from "./chunks.ts";

test("chunk size is 8 MiB of string", () => {
  assert.equal(CHUNK_CHARS, 8 * 1024 * 1024);
});

test("chunks round-trip a 20 MB string", () => {
  const unit = "<p>héllo wörld 😀 日本語</p>\n";
  const s = unit.repeat(Math.ceil((20 * 1024 * 1024) / unit.length));
  const parts = splitString(s);
  assert.ok(parts.length >= 3, `want >= 3 parts, got ${parts.length}`);
  for (const p of parts) assert.ok(p.length <= CHUNK_CHARS);
  assert.equal(joinChunks(parts), s);
});

test("empty string is one empty chunk", () => {
  assert.deepEqual(splitString(""), [""]);
  assert.equal(joinChunks([""]), "");
});

test("short string is a single chunk", () => {
  assert.deepEqual(splitString("abc", 10), ["abc"]);
});

test("exact multiple splits evenly", () => {
  assert.deepEqual(splitString("abcdef", 3), ["abc", "def"]);
});

test("never splits a surrogate pair", () => {
  // "a😀" is a, high, low: a boundary at 2 would separate the pair.
  const s = "a😀b😀😀c";
  const parts = splitString(s, 2);
  assert.equal(joinChunks(parts), s);
  for (const p of parts) {
    assert.ok(p.length > 0 && p.length <= 2);
    const last = p.charCodeAt(p.length - 1);
    assert.ok(
      !(last >= 0xd800 && last <= 0xdbff),
      `chunk ends with high surrogate: ${JSON.stringify(p)}`,
    );
    const first = p.charCodeAt(0);
    assert.ok(
      !(first >= 0xdc00 && first <= 0xdfff),
      `chunk starts with low surrogate: ${JSON.stringify(p)}`,
    );
    // Each chunk is valid UTF-16 on its own, so it survives JSON/UTF-8 encoding.
    assert.equal(new TextDecoder().decode(new TextEncoder().encode(p)), p);
  }
});

test("unicode across default-size boundaries survives", () => {
  // Place an emoji so its high surrogate is the last unit of the first chunk.
  const s = "x".repeat(CHUNK_CHARS - 1) + "😀" + "y".repeat(10);
  const parts = splitString(s);
  assert.equal(parts.length, 2);
  assert.equal(parts[0]?.length, CHUNK_CHARS - 1);
  assert.ok(parts[1]?.startsWith("😀"));
  assert.equal(joinChunks(parts), s);
});

test("rejects a chunk size below 2", () => {
  assert.throws(() => splitString("abc", 1), RangeError);
  assert.throws(() => splitString("abc", 0), RangeError);
});

test("base64 round-trips small inputs", () => {
  assert.equal(bytesToBase64(new Uint8Array()), "");
  assert.equal(bytesToBase64(new TextEncoder().encode("hello")), "aGVsbG8=");
  assert.deepEqual(base64ToBytes("aGVsbG8="), new TextEncoder().encode("hello"));
  const all = new Uint8Array(256).map((_, i) => i);
  assert.deepEqual(base64ToBytes(bytesToBase64(all)), all);
});

test("base64 round-trips 5 MB without overflowing the stack", () => {
  const bytes = new Uint8Array(5 * 1024 * 1024 + 7);
  for (let i = 0; i < bytes.length; i++) bytes[i] = (i * 31 + (i >> 8)) & 0xff;
  const b64 = bytesToBase64(bytes);
  assert.equal(b64, Buffer.from(bytes).toString("base64"));
  assert.deepEqual(base64ToBytes(b64), bytes);
});

test("ChunkAssembler joins ordered chunks", async () => {
  const { ChunkAssembler } = await import("./chunks.ts");
  const a = new ChunkAssembler();
  a.add(0, 2, "ab");
  a.add(1, 2, "cd");
  assert.equal(a.finish(), "abcd");
});

test("ChunkAssembler rejects bad order, total change and incomplete", async () => {
  const { ChunkAssembler } = await import("./chunks.ts");
  const skip = new ChunkAssembler();
  assert.throws(() => skip.add(1, 2, "x"), /out of order/);
  const dup = new ChunkAssembler();
  dup.add(0, 2, "x");
  assert.throws(() => dup.add(0, 2, "x"), /out of order/);
  const tot = new ChunkAssembler();
  tot.add(0, 2, "x");
  assert.throws(() => tot.add(1, 3, "x"), /total changed/);
  const inc = new ChunkAssembler();
  inc.add(0, 2, "x");
  assert.throws(() => inc.finish(), /incomplete/);
  assert.throws(() => new ChunkAssembler().finish(), /incomplete/);
  assert.throws(() => new ChunkAssembler().add(0, 0, "x"), /invalid chunk total/);
});
