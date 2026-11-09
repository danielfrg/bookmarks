import test from "node:test";
import assert from "node:assert/strict";
import { displayTitle, groupTags, hostOf, toggleTagInQuery } from "./query.ts";

test("toggleTagInQuery adds and removes tag tokens", () => {
  assert.equal(toggleTagInQuery("", "go"), "#go");
  assert.equal(toggleTagInQuery("foo", "go"), "foo #go");
  assert.equal(toggleTagInQuery("foo #Go bar", "go"), "foo bar");
  assert.equal(toggleTagInQuery("foo #gopher", "go"), "foo #gopher #go");
  assert.equal(toggleTagInQuery("  foo   #GO  ", "Go"), "foo");
  assert.equal(toggleTagInQuery("  foo   bar ", "go"), "foo bar #go");
  assert.equal(toggleTagInQuery("#go #go x", "go"), "x");
});

test("displayTitle falls back to the URL", () => {
  assert.equal(displayTitle({ title: "Hi", url: "https://a.test" }), "Hi");
  assert.equal(displayTitle({ title: "  ", url: "https://a.test" }), "https://a.test");
});

test("hostOf strips www and tolerates invalid URLs", () => {
  assert.equal(hostOf("https://www.example.com/x"), "example.com");
  assert.equal(hostOf("http://sub.example.com:8080/"), "sub.example.com");
  assert.equal(hostOf("not a url"), "");
});

test("groupTags buckets A-Z with a # bucket for non-letters", () => {
  const t = (id: number, name: string) => ({ id, name, count: 1 });
  const groups = groupTags([
    t(1, "zeta"),
    t(2, "Alpha"),
    t(3, "2fa"),
    t(4, "apple"),
    t(5, "élan"),
    t(6, "_x"),
  ]);
  assert.deepEqual(
    groups.map((g) => [g.letter, g.tags.map((x) => x.name)]),
    [
      ["A", ["Alpha", "apple"]],
      ["E", ["élan"]],
      ["Z", ["zeta"]],
      ["#", ["2fa", "_x"]],
    ],
  );
});
