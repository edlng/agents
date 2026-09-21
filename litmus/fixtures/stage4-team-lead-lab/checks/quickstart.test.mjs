import assert from "node:assert/strict";
import fs from "node:fs";
import test from "node:test";

test("quickstart contains runnable cross-language onboarding", () => {
  const text = fs.readFileSync("docs/quickstart.md", "utf8");
  for (const pattern of [
    /^# Quickstart/m,
    /Prerequisites/i,
    /node --test node\/tag-normalizer\/test\.mjs/,
    /python3 python\/retry_after\/test_retry_after\.py/,
    /go test \.\/go\/slugify/,
    /Expected output/i,
    /Troubleshooting/i,
  ]) {
    assert.match(text, pattern);
  }
});
