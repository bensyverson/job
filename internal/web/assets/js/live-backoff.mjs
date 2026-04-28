/*
  Exponential backoff for SSE reconnect attempts.

  EventSource's built-in reconnect retries roughly every 3s with no
  ceiling, so a long server outage can mean 1200 retries per hour from
  every open dashboard. This helper produces a doubling delay capped at
  capMs, with optional jitter so a fleet of dashboards doesn't all
  reconnect on the same tick.

  Pure function, no DOM/timer dependencies — exported separately from
  live.js so the math is unit-testable under `node --test`.
*/

const DEFAULTS = {
  baseMs: 1000,
  capMs: 30000,
  jitter: 0.2,
  rand: Math.random,
};

export function computeBackoff(attempts, opts = {}) {
  const o = { ...DEFAULTS, ...opts };
  const n = Math.max(0, attempts | 0);
  const raw = o.baseMs * Math.pow(2, n);
  const capped = Math.min(o.capMs, raw);
  const j = o.jitter * (2 * o.rand() - 1); // ∈ [-jitter, +jitter]
  const ms = Math.round(capped * (1 + j));
  return Math.max(Math.floor(o.baseMs / 2), ms);
}
