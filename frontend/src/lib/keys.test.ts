import test from "node:test";
import assert from "node:assert/strict";
import { shortcutFor } from "./keys.ts";

const ev = (key: string, extra: Record<string, unknown> = {}) =>
  ({
    key,
    metaKey: false,
    ctrlKey: false,
    altKey: false,
    shiftKey: false,
    target: null,
    ...extra,
  }) as never;
// Minimal element stand-in: tagName plus closest()/isContentEditable.
const el = (tagName: string, opts: { editable?: boolean; inDialog?: boolean } = {}) =>
  ({
    tagName,
    isContentEditable: !!opts.editable,
    closest: (sel: string) => (opts.inDialog && sel.includes("[role=dialog]") ? {} : null),
  }) as unknown as EventTarget;

test("maps each key", () => {
  const cases: [string, string][] = [
    ["n", "new"],
    ["s", "search"],
    ["/", "search"],
    ["j", "next"],
    ["ArrowDown", "next"],
    ["k", "prev"],
    ["ArrowUp", "prev"],
    ["Enter", "open"],
    ["o", "open"],
    ["v", "view"],
    ["e", "edit"],
  ];
  for (const [key, want] of cases) assert.equal(shortcutFor(ev(key)), want, key);
});

test("? is help, with or without shift", () => {
  assert.equal(shortcutFor(ev("?", { shiftKey: true })), "help");
  assert.equal(shortcutFor(ev("?")), "help");
});

test("unknown keys are null", () => {
  assert.equal(shortcutFor(ev("x")), null);
});

test("ignored in form fields, contenteditable and dialogs", () => {
  for (const tag of ["INPUT", "TEXTAREA", "SELECT"])
    assert.equal(shortcutFor(ev("n", { target: el(tag) })), null, tag);
  assert.equal(shortcutFor(ev("n", { target: el("DIV", { editable: true }) })), null);
  assert.equal(shortcutFor(ev("n", { target: el("BUTTON", { inDialog: true }) })), null);
  assert.equal(shortcutFor(ev("n", { target: el("BUTTON") })), "new");
});

test("ignored with meta/ctrl/alt; shift only for ?", () => {
  for (const m of ["metaKey", "ctrlKey", "altKey"])
    assert.equal(shortcutFor(ev("n", { [m]: true })), null, m);
  assert.equal(shortcutFor(ev("?", { ctrlKey: true })), null);
  assert.equal(shortcutFor(ev("n", { shiftKey: true })), null);
  assert.equal(shortcutFor(ev("N", { shiftKey: true })), null);
});
