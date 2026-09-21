import assert from "node:assert/strict";
import test from "node:test";
import { parseDuration } from "./index.mjs";

test("parses compact and spaced duration components", () => {
  assert.equal(parseDuration("1h 30m"), 5_400_000);
  assert.equal(parseDuration("2d4h5s"), 187_205_000);
  assert.equal(parseDuration("250ms"), 250);
});

test("rejects empty, duplicate, negative, and unknown units", () => {
  for (const value of ["", "1h 2h", "-1m", "4fortnights", "12"]) {
    assert.throws(() => parseDuration(value), /duration/i);
  }
});
