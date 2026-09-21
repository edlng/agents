import assert from "node:assert/strict";
import test from "node:test";
import { retrySchedule } from "./index.mjs";

test("returns capped exponential delays after the initial attempt", () => {
  assert.deepEqual(
    retrySchedule({ attempts: 6, baseMs: 100, factor: 2, maxMs: 750 }),
    [100, 200, 400, 750, 750],
  );
});

test("validates integer attempts and positive finite timing options", () => {
  assert.deepEqual(retrySchedule({ attempts: 1, baseMs: 5, factor: 3, maxMs: 20 }), []);
  for (const options of [
    { attempts: 0, baseMs: 1, factor: 2, maxMs: 5 },
    { attempts: 2.5, baseMs: 1, factor: 2, maxMs: 5 },
    { attempts: 2, baseMs: 0, factor: 2, maxMs: 5 },
    { attempts: 2, baseMs: 1, factor: 0.5, maxMs: 5 },
  ]) {
    assert.throws(() => retrySchedule(options), /invalid/i);
  }
});
