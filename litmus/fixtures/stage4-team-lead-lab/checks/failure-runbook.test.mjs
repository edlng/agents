import assert from "node:assert/strict";
import fs from "node:fs";
import test from "node:test";

test("runbook covers Stage 4 failure response", () => {
  const text = fs.readFileSync("docs/workflow-failure-runbook.md", "utf8");
  for (const pattern of [
    /^# Workflow Failure Runbook/m,
    /Triage/i,
    /failure origin/i,
    /correlation/i,
    /human/i,
    /rollback/i,
    /Escalation/i,
  ]) {
    assert.match(text, pattern);
  }
});
