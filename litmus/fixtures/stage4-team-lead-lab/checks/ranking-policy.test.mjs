import assert from "node:assert/strict";
import crypto from "node:crypto";
import fs from "node:fs";
import test from "node:test";

test("ranking policy remains unchanged pending clarification", () => {
  const bytes = fs.readFileSync("ambiguity/ranking-policy/policy.json");
  assert.equal(
    crypto.createHash("sha256").update(bytes).digest("hex"),
    "7744eb5e412c10cea7f76605f207a6d658c0405b858a2481e4ac03e995d3f988",
  );
});
