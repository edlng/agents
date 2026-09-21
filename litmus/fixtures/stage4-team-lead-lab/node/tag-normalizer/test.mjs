import assert from "node:assert/strict";
import test from "node:test";
import { normalizeTags } from "./index.mjs";

test("normalizes, filters, and de-duplicates tags in first-seen order", () => {
  assert.deepEqual(
    normalizeTags(["  High Priority ", "high-priority", "API_v2", "", "bad!", "api_v2"]),
    ["high-priority", "api_v2"],
  );
});

test("does not mutate the input and rejects non-arrays", () => {
  const input = [" One "];
  assert.deepEqual(normalizeTags(input), ["one"]);
  assert.deepEqual(input, [" One "]);
  assert.throws(() => normalizeTags("one"), TypeError);
});
