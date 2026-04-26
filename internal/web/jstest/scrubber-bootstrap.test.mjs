// Tests for internal/web/assets/js/scrubber-bootstrap.mjs.
//
// The bootstrap glues the SSR-emitted JSON island to ReplayBuffer:
// parse the island, wrap it in an initialFrame, configure a fetcher
// against /events, and stash the buffer on a known global so other
// modules (the scrubber pill, the timeline) can grab it.
//
// We test the pure pieces here. The DOM-read path
// (document.getElementById('initial-frame')) is a one-liner over
// parseInitialFrameJSON; auto-init in the browser context isn't
// covered (that needs a JS DOM, which we don't ship as a dep).

import { test } from "node:test";
import assert from "node:assert/strict";

import {
  parseInitialFrameJSON,
  buildEventsFetcher,
  createScrubber,
} from "../assets/js/scrubber-bootstrap.mjs";

// --- parseInitialFrameJSON ---

test("parseInitialFrameJSON: valid payload returns the parsed object", () => {
  const raw = JSON.stringify({
    headEventId: 5,
    tasks: [{ shortId: "ABC12", title: "T", status: "available", sortOrder: 0 }],
    blocks: [],
    claims: [],
  });
  const payload = parseInitialFrameJSON(raw);
  assert.equal(payload.headEventId, 5);
  assert.equal(payload.tasks.length, 1);
});

test("parseInitialFrameJSON: empty / null input returns null", () => {
  assert.equal(parseInitialFrameJSON(""), null);
  assert.equal(parseInitialFrameJSON(null), null);
  assert.equal(parseInitialFrameJSON(undefined), null);
});

test("parseInitialFrameJSON: malformed JSON returns null (does not throw)", () => {
  assert.equal(parseInitialFrameJSON("not json"), null);
  assert.equal(parseInitialFrameJSON("{ broken"), null);
});

// --- createScrubber ---

test("createScrubber: returns null when payload is null", () => {
  assert.equal(createScrubber(null, async () => []), null);
});

test("createScrubber: returns a ReplayBuffer wired to the head event", async () => {
  const payload = {
    headEventId: 0,
    tasks: [],
    blocks: [],
    claims: [],
  };
  const buf = createScrubber(payload, async () => []);
  assert.notEqual(buf, null);
  // Head lookup must not call the fetcher and must return the seed.
  const head = await buf.frameAt(0);
  assert.equal(head.eventId, 0);
});

// --- buildEventsFetcher ---

test("buildEventsFetcher: composes the URL with since and limit", async () => {
  // Capture the URL the fetcher requests against a stub fetch.
  const urls = [];
  const stubFetch = async (input) => {
    urls.push(typeof input === "string" ? input : input.toString());
    return new Response("[]", { status: 200, headers: { "content-type": "application/json" } });
  };
  const fetcher = buildEventsFetcher({ baseURL: "http://example.test/events", fetch: stubFetch });
  await fetcher({ since: 42, limit: 10 });

  assert.equal(urls.length, 1);
  const url = new URL(urls[0]);
  assert.equal(url.searchParams.get("since"), "42");
  assert.equal(url.searchParams.get("limit"), "10");
});

test("buildEventsFetcher: omits since when zero (matches /events default)", async () => {
  const urls = [];
  const stubFetch = async (input) => {
    urls.push(typeof input === "string" ? input : input.toString());
    return new Response("[]", { status: 200, headers: { "content-type": "application/json" } });
  };
  const fetcher = buildEventsFetcher({ baseURL: "http://example.test/events", fetch: stubFetch });
  await fetcher({ limit: 5 });

  const url = new URL(urls[0]);
  assert.equal(url.searchParams.has("since"), false);
});

test("buildEventsFetcher: surfaces non-2xx responses as thrown errors", async () => {
  const stubFetch = async () =>
    new Response("nope", { status: 500 });
  const fetcher = buildEventsFetcher({ baseURL: "http://example.test/events", fetch: stubFetch });
  await assert.rejects(() => fetcher({ since: 0, limit: 10 }), /500/);
});
