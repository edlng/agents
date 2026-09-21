import assert from "node:assert/strict";
import test from "node:test";
import { safeMerge } from "./index.mjs";

test("recursively merges plain objects without mutating inputs", () => {
  const base = { service: { timeout: 10, retries: 2 }, enabled: true };
  const patch = { service: { timeout: 20 }, label: "lab" };
  assert.deepEqual(safeMerge(base, patch), {
    service: { timeout: 20, retries: 2 },
    enabled: true,
    label: "lab",
  });
  assert.equal(base.service.timeout, 10);
});

test("blocks prototype-pollution keys at every depth", () => {
  const patch = JSON.parse(
    '{"__proto__":{"polluted":true},"nested":{"constructor":{"prototype":{"polluted":true}},"safe":1}}',
  );
  assert.deepEqual(safeMerge({}, patch), { nested: { safe: 1 } });
  assert.equal({}.polluted, undefined);
});

test("replaces arrays instead of merging their indexes", () => {
  const patch = { values: [{ nested: true }] };
  const result = safeMerge({ values: [1, 2] }, patch);
  assert.deepEqual(result, patch);
  assert.notEqual(result.values, patch.values);
  assert.notEqual(result.values[0], patch.values[0]);
});

test("does not alias mutable non-plain replacement values", () => {
  class Box {
    constructor(value) {
      this.value = value;
    }
  }
  const date = new Date("2026-09-15T12:00:00.000Z");
  const box = new Box({ nested: true });
  const nullPrototype = Object.assign(Object.create(null), { nested: { value: 1 } });
  const result = safeMerge({}, { date, box, nullPrototype });

  assert.notEqual(result.date, date);
  assert.equal(result.date.getTime(), date.getTime());
  assert.notEqual(result.box, box);
  assert.equal(Object.getPrototypeOf(result.box), Box.prototype);
  assert.notEqual(result.box.value, box.value);
  assert.notEqual(result.nullPrototype, nullPrototype);
  assert.equal(Object.getPrototypeOf(result.nullPrototype), null);
  assert.notEqual(result.nullPrototype.nested, nullPrototype.nested);
});
