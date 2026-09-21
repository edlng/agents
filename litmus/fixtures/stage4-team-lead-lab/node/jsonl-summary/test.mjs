import assert from "node:assert/strict";
import test from "node:test";
import { summarizeJsonl } from "./index.mjs";

test("summarizes nonblank JSONL records with stable currency rounding", () => {
  const input = [
    '{"status":"pass","cost_usd":0.105}',
    "",
    "  ",
    '{"status":"fail","cost_usd":0.205}',
    '{"status":"pass"}',
  ].join("\r\n");
  assert.deepEqual(summarizeJsonl(input), {
    runs: 3,
    passed: 2,
    failed: 1,
    costUsd: 0.31,
  });
});

test("reports the one-based line number for malformed or invalid records", () => {
  assert.throws(() => summarizeJsonl('{"status":"pass"}\nnope'), /line 2/i);
  assert.throws(() => summarizeJsonl('{"status":"unknown"}'), /line 1/i);
});
