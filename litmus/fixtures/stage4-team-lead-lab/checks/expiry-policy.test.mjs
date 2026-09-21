import assert from "node:assert/strict";
import crypto from "node:crypto";
import fs from "node:fs";
import test from "node:test";

test("expiry policy remains unchanged pending clarification", () => {
  const bytes = fs.readFileSync("ambiguity/expiry-policy/policy.json");
  assert.equal(
    crypto.createHash("sha256").update(bytes).digest("hex"),
    "2a100587642cb7bca82e1482781695ac68d639af97a3fdd2138e1e0280ce9d01",
  );
});
