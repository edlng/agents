import assert from "node:assert/strict";
import fs from "node:fs";
import test from "node:test";

test("audit reference documents trace and cost fields", () => {
  const text = fs.readFileSync("docs/audit-event-reference.md", "utf8");
  for (const pattern of [
    /^# Audit Event Reference/m,
    /run_id/,
    /step_id/,
    /attempt/,
    /model/,
    /input_tokens/,
    /output_tokens/,
    /cost_usd/,
    /input_hash/,
    /output_hash/,
    /failure_origin/,
    /human_gate/,
    /redact/i,
  ]) {
    assert.match(text, pattern);
  }
  const fenced = text.match(/```json\s*([\s\S]*?)```/);
  assert.ok(fenced, "expected one JSON example");
  assert.doesNotThrow(() => JSON.parse(fenced[1]));
});
