/*
  Scrubber math, side-effect-free.

  Maps cursor x-fraction (0..1 across the strip) to event ids and
  back, using event timestamps as the time axis. The strip's left
  edge is "24h ago" and the right edge is "now"; events outside that
  window aren't navigable through the strip but are still reachable
  by URL.

  Also computes density bars for the histogram and formats the
  history banner string. All shared with scrubber-pill.mjs and any
  future module that needs to project events onto the strip.
*/

const ONE_DAY_MS = 24 * 60 * 60 * 1000;

// xToEventId maps a cursor x-fraction (0 = 24h ago, 1 = now) to the
// largest event id whose created_at falls at or before that moment.
// Empty events arrays return 0; x past the most recent event clamps
// to head; x before the window's oldest event clamps to its id.
//
// Events are expected to be sorted ascending by created_at — the
// /events endpoint returns them in that order.
export function xToEventId(xFrac, events, nowMs, windowMs = ONE_DAY_MS) {
  if (events.length === 0) return 0;
  const targetMs = nowMs - (1 - xFrac) * windowMs;
  // Binary search for the rightmost event with created_at*1000 <= targetMs.
  let lo = 0;
  let hi = events.length;
  while (lo < hi) {
    const mid = (lo + hi) >>> 1;
    if (events[mid].created_at * 1000 <= targetMs) lo = mid + 1;
    else hi = mid;
  }
  if (lo === 0) return events[0].id;
  return events[lo - 1].id;
}

// eventIdToX is the inverse: returns the x-fraction of the cursor
// that corresponds to the given event id, by looking up the event's
// created_at and projecting it into the [24h ago, now] window. Events
// outside the window clamp to 0 or 1; unknown ids return 1.
export function eventIdToX(eventId, events, nowMs, windowMs = ONE_DAY_MS) {
  const event = events.find((e) => e.id === eventId);
  if (!event) return 1;
  const ageMs = nowMs - event.created_at * 1000;
  if (ageMs <= 0) return 1;
  if (ageMs >= windowMs) return 0;
  return 1 - ageMs / windowMs;
}

// computeDensityBars buckets events into N bars across the window
// and returns each bar's relative height as an integer percent. The
// busiest bucket is normalized to 100; empty buckets stay at 0.
// Output is fixed-length (bucketCount) so the caller can render bars
// even when the DB has zero events.
export function computeDensityBars(
  events,
  nowMs,
  bucketCount = 60,
  windowMs = ONE_DAY_MS,
) {
  const buckets = new Array(bucketCount).fill(0);
  const bucketSizeMs = windowMs / bucketCount;
  for (const e of events) {
    const ageMs = nowMs - e.created_at * 1000;
    if (ageMs < 0 || ageMs >= windowMs) continue;
    // Bucket 0 = oldest (24h ago); bucket N-1 = newest (now).
    let idx = Math.floor((windowMs - ageMs) / bucketSizeMs);
    if (idx < 0) idx = 0;
    if (idx >= bucketCount) idx = bucketCount - 1;
    buckets[idx]++;
  }
  let max = 0;
  for (const c of buckets) if (c > max) max = c;
  if (max === 0) return buckets.map(() => 0);
  return buckets.map((c) => Math.round((c / max) * 100));
}

// formatAge formats a millisecond duration as a compact relative
// string ("45s ago", "5m ago", "3h ago", "2d ago"). Negative inputs
// are treated as zero; matches the convention used in render.RelativeTime
// on the Go side so the wall-clock + relative align across the page.
export function formatAge(ageMs) {
  const sec = Math.max(0, Math.floor(ageMs / 1000));
  if (sec < 60) return `${sec}s ago`;
  const min = Math.floor(sec / 60);
  if (min < 60) return `${min}m ago`;
  const h = Math.floor(min / 60);
  if (h < 24) return `${h}h ago`;
  const d = Math.floor(h / 24);
  return `${d}d ago`;
}

// formatHistoryBannerText builds the "?at=N · Ns ago · YYYY-MM-DD HH:MM:SS"
// string the history banner shows. UTC timestamp keeps the wire
// representation consistent regardless of viewer locale; the relative
// part follows the local clock-derived `ageMs`.
export function formatHistoryBannerText(event, nowMs) {
  const ageMs = nowMs - event.created_at * 1000;
  const age = formatAge(ageMs);
  const iso = new Date(event.created_at * 1000).toISOString().replace("T", " ").slice(0, 19);
  return `?at=${event.id} · ${age} · ${iso}`;
}
