import { test } from "node:test";
import assert from "node:assert/strict";
import { StageTracker, captureTimeoutMessage } from "./capture-stage.ts";

test("describes single-file-core progress", () => {
  const t = new StageTracker();
  assert.equal(t.update({ type: "page-loading" }), "reading the page");
  assert.equal(t.update({ type: "page-loaded" }), null);
  assert.equal(
    t.update({ type: "resources-initialized", detail: { max: 3 } }),
    "fetching resources (0/3)",
  );
  t.update({ type: "resource-loaded" });
  assert.equal(t.update({ type: "resource-loaded" }), "fetching resources (2/3)");
  assert.equal(
    t.update({ type: "stage-started", detail: { step: 2 } }),
    "processing the page (step 3 of 6)",
  );
  assert.equal(t.update({ type: "stage-ended", detail: { step: 2 } }), null);
  assert.equal(t.update({ type: "page-ended" }), "serializing the page");
});

test("timeout message names the stage and how long it has been stuck", () => {
  assert.equal(
    captureTimeoutMessage({
      stage: "fetching resources (2/3)",
      elapsedMs: 120_000,
      sinceProgressMs: 95_400,
    }),
    "capture timed out after 120 s while fetching resources (2/3) (last progress 95 s ago)",
  );
});
