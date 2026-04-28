import { test } from "node:test";
import assert from "node:assert/strict";

import { computeBackoff } from "../assets/js/live-backoff.mjs";

// Deterministic helper: pin the random source so we can assert the
// exact delay including jitter.
function fixedRand(value) {
  return () => value;
}

test("computeBackoff: first attempt returns base delay (no jitter at 0.0)", () => {
  const ms = computeBackoff(0, { baseMs: 1000, capMs: 30000, jitter: 0, rand: fixedRand(0) });
  assert.equal(ms, 1000);
});

test("computeBackoff: doubles each attempt up to the cap", () => {
  const opts = { baseMs: 1000, capMs: 30000, jitter: 0, rand: fixedRand(0) };
  assert.equal(computeBackoff(0, opts), 1000);
  assert.equal(computeBackoff(1, opts), 2000);
  assert.equal(computeBackoff(2, opts), 4000);
  assert.equal(computeBackoff(3, opts), 8000);
  assert.equal(computeBackoff(4, opts), 16000);
});

test("computeBackoff: clamps to capMs once doubling overshoots", () => {
  const opts = { baseMs: 1000, capMs: 30000, jitter: 0, rand: fixedRand(0) };
  assert.equal(computeBackoff(5, opts), 30000); // 32000 → capped
  assert.equal(computeBackoff(10, opts), 30000);
  assert.equal(computeBackoff(50, opts), 30000);
});

test("computeBackoff: jitter adds ±jitter*delay using rand()", () => {
  // rand=0 → multiplier 1 - jitter
  // rand=1 → multiplier 1 + jitter
  // rand=0.5 → multiplier 1 (no shift)
  const base = { baseMs: 1000, capMs: 30000, jitter: 0.2 };
  assert.equal(computeBackoff(0, { ...base, rand: fixedRand(0.0) }), 800);
  assert.equal(computeBackoff(0, { ...base, rand: fixedRand(0.5) }), 1000);
  assert.equal(computeBackoff(0, { ...base, rand: fixedRand(1.0) }), 1200);
});

test("computeBackoff: never returns less than baseMs/2 even with jitter pulling down", () => {
  const ms = computeBackoff(0, {
    baseMs: 100,
    capMs: 30000,
    jitter: 0.99,
    rand: fixedRand(0),
  });
  assert.ok(ms >= 50, `expected floor at baseMs/2, got ${ms}`);
});

test("computeBackoff: defaults are reasonable", () => {
  // Should not throw with no opts.
  const ms = computeBackoff(0);
  assert.ok(ms >= 0);
});

test("computeBackoff: negative attempts treated as zero", () => {
  const opts = { baseMs: 1000, capMs: 30000, jitter: 0, rand: fixedRand(0) };
  assert.equal(computeBackoff(-1, opts), 1000);
  assert.equal(computeBackoff(-100, opts), 1000);
});
