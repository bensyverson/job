// Tests for internal/web/assets/js/scrubber-cursor.mjs.
//
// Pure math the scrubber UI needs: convert between cursor x-fraction
// (0..1 across the strip) and event id, bin events into density
// buckets for the histogram bars, format the history banner text.
// All side-effect-free; the DOM-binding layer lives in
// scrubber-pill.mjs and isn't covered here.

import { test } from "node:test";
import assert from "node:assert/strict";

import {
  xToEventId,
  eventIdToX,
  computeDensityBars,
  formatHistoryBannerText,
  formatAge,
} from "../assets/js/scrubber-cursor.mjs";

// Helper: build an events array with a fixed offset back from `now`.
// Each entry's created_at is an integer second; the array is sorted
// ascending by created_at, matching how the server returns events.
function eventsAt(now, ageSecondsList) {
  return ageSecondsList.map((ageSec, i) => ({
    id: i + 1,
    created_at: Math.floor((now - ageSec * 1000) / 1000),
  }));
}

const NOW = 1735000000_000; // arbitrary fixed millis-since-epoch

// --- xToEventId ---

test("xToEventId: empty events returns 0", () => {
  assert.equal(xToEventId(0.5, [], NOW), 0);
});

test("xToEventId: x=1 returns the most recent event id", () => {
  // Three events at 23h, 12h, 1h ago.
  const events = eventsAt(NOW, [23 * 3600, 12 * 3600, 1 * 3600]);
  assert.equal(xToEventId(1, events, NOW), 3);
});

test("xToEventId: x=0 returns the oldest event in the window (or before)", () => {
  // Three events at 23h, 12h, 1h ago. x=0 means "24h ago" — first
  // event is the closest match.
  const events = eventsAt(NOW, [23 * 3600, 12 * 3600, 1 * 3600]);
  assert.equal(xToEventId(0, events, NOW), 1);
});

test("xToEventId: midpoint resolves to the event just before that time", () => {
  // 24h window: x=0.5 means "12h ago". With events at 23h, 12h, 1h,
  // the largest event id with created_at <= (now - 12h) is event 2.
  const events = eventsAt(NOW, [23 * 3600, 12 * 3600, 1 * 3600]);
  assert.equal(xToEventId(0.5, events, NOW), 2);
});

test("xToEventId: x past the most-recent event clamps to head", () => {
  const events = eventsAt(NOW, [10 * 3600, 5 * 3600]);
  assert.equal(xToEventId(1, events, NOW), 2);
});

// --- eventIdToX ---

test("eventIdToX: event at now returns 1", () => {
  const events = eventsAt(NOW, [0]);
  assert.equal(eventIdToX(1, events, NOW), 1);
});

test("eventIdToX: event 24h+ ago returns 0", () => {
  const events = eventsAt(NOW, [25 * 3600]);
  assert.equal(eventIdToX(1, events, NOW), 0);
});

test("eventIdToX: event mid-window returns mid-x", () => {
  const events = eventsAt(NOW, [12 * 3600]);
  // 12h ago in a 24h window = age 12h / 24h = 0.5; x = 1 - 0.5 = 0.5.
  assert.equal(eventIdToX(1, events, NOW), 0.5);
});

test("eventIdToX: unknown event id returns 1 (clamps to head)", () => {
  const events = eventsAt(NOW, [1 * 3600]);
  assert.equal(eventIdToX(999, events, NOW), 1);
});

// --- computeDensityBars ---

test("computeDensityBars: empty events returns all-zero bars", () => {
  const bars = computeDensityBars([], NOW, 60);
  assert.equal(bars.length, 60);
  for (const v of bars) assert.equal(v, 0);
});

test("computeDensityBars: all events in last bucket peak that bucket", () => {
  // Three events all within the last minute. With 60 buckets over
  // 24h, that's the last bucket (index 59).
  const events = eventsAt(NOW, [10, 20, 30]); // seconds ago
  const bars = computeDensityBars(events, NOW, 60);
  assert.equal(bars[59], 100, "last bucket should peak at 100%");
  for (let i = 0; i < 59; i++) {
    assert.equal(bars[i], 0, `bucket ${i} should be 0`);
  }
});

test("computeDensityBars: events outside the 24h window are excluded", () => {
  const events = eventsAt(NOW, [25 * 3600, 30 * 3600, 1 * 3600]);
  const bars = computeDensityBars(events, NOW, 24);
  // Only the 1h-ago event lands in a bucket.
  const total = bars.reduce((a, b) => a + (b > 0 ? 1 : 0), 0);
  assert.equal(total, 1, "exactly one non-empty bucket expected");
});

// --- formatAge / formatHistoryBannerText ---

test("formatAge: covers seconds, minutes, hours, days", () => {
  assert.equal(formatAge(45_000), "45s ago");
  assert.equal(formatAge(5 * 60_000), "5m ago");
  assert.equal(formatAge(3 * 3600_000), "3h ago");
  assert.equal(formatAge(2 * 86400_000), "2d ago");
});

test("formatHistoryBannerText: composes id, age, and ISO timestamp", () => {
  const event = { id: 1234, created_at: Math.floor((NOW - 6 * 3600_000) / 1000) };
  const text = formatHistoryBannerText(event, NOW);
  assert.match(text, /\?at=1234/);
  assert.match(text, /6h ago/);
  assert.match(text, /\d{4}-\d{2}-\d{2}/);
});
