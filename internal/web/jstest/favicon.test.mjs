// Tests for internal/web/assets/js/favicon.mjs.
//
// The dynamic favicon reflects four mutually-exclusive states:
//
//   broken — SSE stream is offline or reconnecting
//   clean  — connected, every top-level task is done/canceled (zero inbox)
//   active — connected, at least one active claim
//   idle   — connected, open work remains, nothing currently claimed
//
// Pure helpers (classifyFaviconState, applyEventToCounters,
// seedFromFrame, renderFaviconSVG, faviconDataURL) are exercised here.
// bindFavicon (the DOM-bound controller) is tested with a fake document
// and a synthetic <live-region>.

import { test } from "node:test";
import assert from "node:assert/strict";

import {
  classifyFaviconState,
  seedFromFrame,
  applyEventToCounters,
  renderFaviconSVG,
  faviconDataURL,
  bindFavicon,
} from "../assets/js/favicon.mjs";

// --- classifyFaviconState ---

test("classifyFaviconState: broken wins over everything when connection is offline", () => {
  assert.equal(classifyFaviconState({ connection: "offline", activeClaims: 5, openTopLevel: 3 }), "broken");
  assert.equal(classifyFaviconState({ connection: "offline", activeClaims: 0, openTopLevel: 0 }), "broken");
});

test("classifyFaviconState: reconnecting also classifies as broken", () => {
  assert.equal(classifyFaviconState({ connection: "reconnecting", activeClaims: 0, openTopLevel: 1 }), "broken");
});

test("classifyFaviconState: connecting (initial) does not yet count as broken", () => {
  // First-load grace: don't flash the warning state before the stream
  // has had a chance to open.
  assert.equal(classifyFaviconState({ connection: "connecting", activeClaims: 0, openTopLevel: 1 }), "idle");
});

test("classifyFaviconState: clean wins over active when openTopLevel is 0", () => {
  // 'Clean slate' is literal — if every top-level is wrapped up, even
  // a stray claim on a leftover child still reads as done overall.
  assert.equal(classifyFaviconState({ connection: "connected", activeClaims: 2, openTopLevel: 0 }), "clean");
});

test("classifyFaviconState: active when claims > 0 and there's still work", () => {
  assert.equal(classifyFaviconState({ connection: "connected", activeClaims: 1, openTopLevel: 4 }), "active");
});

test("classifyFaviconState: idle when connected, open work remains, no claims", () => {
  assert.equal(classifyFaviconState({ connection: "connected", activeClaims: 0, openTopLevel: 7 }), "idle");
});

// --- seedFromFrame ---

const taskState = (over = {}) => ({
  shortId: "abc",
  title: "t",
  status: "available",
  parentShortId: null,
  ...over,
});

test("seedFromFrame: counts top-level open tasks; ignores children", () => {
  const s = seedFromFrame({
    tasks: [
      taskState({ shortId: "r1", parentShortId: null, status: "available" }),
      taskState({ shortId: "r2", parentShortId: null, status: "claimed" }),
      taskState({ shortId: "r3", parentShortId: null, status: "done" }),
      taskState({ shortId: "c1", parentShortId: "r1", status: "available" }),
    ],
    claims: [],
  });
  assert.equal(s.openTopLevel, 2);
  // topLevelStatus tracks all roots, regardless of status, so reopened
  // events on previously-closed roots can be applied correctly.
  assert.equal(s.topLevelStatus.size, 3);
});

test("seedFromFrame: activeClaims = claims.length", () => {
  const s = seedFromFrame({
    tasks: [],
    claims: [
      { shortId: "x", claimedBy: "a" },
      { shortId: "y", claimedBy: "b" },
    ],
  });
  assert.equal(s.activeClaims, 2);
});

test("seedFromFrame: missing payload returns zero counters with empty map", () => {
  const s = seedFromFrame(null);
  assert.equal(s.activeClaims, 0);
  assert.equal(s.openTopLevel, 0);
  assert.equal(s.topLevelStatus.size, 0);
});

// --- applyEventToCounters ---

function freshState(over = {}) {
  return {
    activeClaims: 0,
    openTopLevel: 0,
    topLevelStatus: new Map(),
    ...over,
  };
}

test("applyEventToCounters: claimed increments active claims", () => {
  const s = freshState();
  applyEventToCounters(s, { type: "claimed", task_id: "x" });
  assert.equal(s.activeClaims, 1);
});

test("applyEventToCounters: released/done/canceled/claim_expired decrement; clamped at 0", () => {
  for (const type of ["released", "done", "canceled", "claim_expired"]) {
    const s = freshState({ activeClaims: 1 });
    applyEventToCounters(s, { type, task_id: "x" });
    assert.equal(s.activeClaims, 0, `type ${type}: 1 -> 0`);
    applyEventToCounters(s, { type, task_id: "x" });
    assert.equal(s.activeClaims, 0, `type ${type}: clamps at 0, no negative`);
  }
});

test("applyEventToCounters: created with no parent_id adds a top-level task", () => {
  const s = freshState();
  applyEventToCounters(s, { type: "created", task_id: "r1", detail: {} });
  assert.equal(s.openTopLevel, 1);
  assert.equal(s.topLevelStatus.get("r1"), "available");
});

test("applyEventToCounters: created with parent_id is ignored for top-level count", () => {
  const s = freshState();
  applyEventToCounters(s, { type: "created", task_id: "c1", detail: { parent_id: "r1" } });
  assert.equal(s.openTopLevel, 0);
  assert.equal(s.topLevelStatus.has("c1"), false);
});

test("applyEventToCounters: done on a top-level task decrements open count", () => {
  const s = freshState();
  applyEventToCounters(s, { type: "created", task_id: "r1", detail: {} });
  applyEventToCounters(s, { type: "done", task_id: "r1" });
  assert.equal(s.openTopLevel, 0);
  assert.equal(s.topLevelStatus.get("r1"), "done");
});

test("applyEventToCounters: done on an already-closed top-level task is a no-op", () => {
  // Defensive: SSE replays could repeat a done event after reconnect.
  const s = freshState();
  applyEventToCounters(s, { type: "created", task_id: "r1", detail: {} });
  applyEventToCounters(s, { type: "done", task_id: "r1" });
  applyEventToCounters(s, { type: "done", task_id: "r1" });
  assert.equal(s.openTopLevel, 0);
});

test("applyEventToCounters: done on a child task does not touch top-level count", () => {
  const s = freshState();
  applyEventToCounters(s, { type: "created", task_id: "r1", detail: {} });
  applyEventToCounters(s, { type: "done", task_id: "c1" }); // never seen as root
  assert.equal(s.openTopLevel, 1);
});

test("applyEventToCounters: reopened on a previously-done top-level task increments open count", () => {
  const s = freshState();
  applyEventToCounters(s, { type: "created", task_id: "r1", detail: {} });
  applyEventToCounters(s, { type: "done", task_id: "r1" });
  applyEventToCounters(s, { type: "reopened", task_id: "r1" });
  assert.equal(s.openTopLevel, 1);
  assert.equal(s.topLevelStatus.get("r1"), "available");
});

test("applyEventToCounters: reopened on a top-level already open is a no-op", () => {
  const s = freshState();
  applyEventToCounters(s, { type: "created", task_id: "r1", detail: {} });
  applyEventToCounters(s, { type: "reopened", task_id: "r1" });
  assert.equal(s.openTopLevel, 1);
});

test("applyEventToCounters: unknown event type leaves state unchanged", () => {
  const s = freshState({ activeClaims: 2, openTopLevel: 3 });
  applyEventToCounters(s, { type: "noted", task_id: "r1" });
  assert.equal(s.activeClaims, 2);
  assert.equal(s.openTopLevel, 3);
});

test("applyEventToCounters: null/undefined event is a safe no-op", () => {
  const s = freshState({ activeClaims: 1 });
  applyEventToCounters(s, null);
  applyEventToCounters(s, undefined);
  applyEventToCounters(s, {});
  assert.equal(s.activeClaims, 1);
});

// --- renderFaviconSVG ---

test("renderFaviconSVG: each state returns a distinct, valid SVG string", () => {
  const seen = new Set();
  for (const state of ["broken", "active", "idle", "clean"]) {
    const svg = renderFaviconSVG(state);
    assert.match(svg, /^<svg[^>]+xmlns="http:\/\/www\.w3\.org\/2000\/svg"/, `state ${state} starts with <svg>`);
    assert.match(svg, /<\/svg>$/, `state ${state} closes <svg>`);
    seen.add(svg);
  }
  assert.equal(seen.size, 4, "each state must render uniquely");
});

test("renderFaviconSVG: clean state contains a checkmark path", () => {
  const svg = renderFaviconSVG("clean");
  assert.match(svg, /<path/);
});

test("renderFaviconSVG: broken state uses the warn signal color", () => {
  const svg = renderFaviconSVG("broken");
  assert.match(svg, /#e8865c/i);
});

test("renderFaviconSVG: active state uses the primary color", () => {
  const svg = renderFaviconSVG("active");
  assert.match(svg, /#3cddc7/i);
});

test("renderFaviconSVG: unknown state falls back to idle, not throwing", () => {
  const svg = renderFaviconSVG("garbage");
  assert.equal(svg, renderFaviconSVG("idle"));
});

// --- faviconDataURL ---

test("faviconDataURL: prefixes with the SVG data URL scheme", () => {
  assert.match(faviconDataURL("idle"), /^data:image\/svg\+xml,/);
});

test("faviconDataURL: percent-encodes characters that break in href context", () => {
  // # is the URL fragment delimiter — leaving it raw breaks the data:
  // URL. The encoded form must round-trip to a recognizable color.
  const url = faviconDataURL("active");
  assert.doesNotMatch(url, /#3cddc7/, "raw '#' must not appear in href");
  assert.match(url, /%233cddc7/i, "color must be percent-encoded");
});

// --- bindFavicon (DOM-bound controller) ---

function fakeLink() {
  return { rel: "icon", type: "", href: "" };
}

function fakeLiveRegion() {
  const handlers = {};
  return {
    addEventListener(type, fn) { handlers[type] = fn; },
    fire(type, detail) {
      if (handlers[type]) handlers[type](new FakeEvent(detail));
    },
  };
}

class FakeEvent {
  constructor(detail) { this.detail = detail; }
}

function fakeDoc(initialFrameJSON) {
  const link = fakeLink();
  const live = fakeLiveRegion();
  return {
    link,
    live,
    querySelector(sel) {
      if (sel === 'link[rel~="icon"]') return link;
      if (sel === "live-region") return live;
      if (sel === "#initial-frame") {
        return initialFrameJSON == null ? null : { textContent: initialFrameJSON };
      }
      return null;
    },
  };
}

test("bindFavicon: with no frame, idle on connect, broken on disconnect", () => {
  const doc = fakeDoc(null);
  bindFavicon({ document: doc });
  // 'connecting' shouldn't change anything from idle initially
  assert.match(doc.link.href, /^data:image\/svg/);
  const idleHref = doc.link.href;

  doc.live.fire("connection", "offline");
  assert.notEqual(doc.link.href, idleHref);

  doc.live.fire("connection", "connected");
  assert.equal(doc.link.href, idleHref);
});

test("bindFavicon: claims drive active vs idle while connected", () => {
  const frame = JSON.stringify({
    headEventId: 0,
    tasks: [{ shortId: "r1", title: "t", status: "available", parentShortId: null }],
    claims: [],
    blocks: [],
  });
  const doc = fakeDoc(frame);
  bindFavicon({ document: doc });
  doc.live.fire("connection", "connected");
  const idleHref = doc.link.href;

  doc.live.fire("event", { type: "claimed", task_id: "r1" });
  assert.notEqual(doc.link.href, idleHref);

  doc.live.fire("event", { type: "done", task_id: "r1" });
  // Now openTopLevel = 0 → clean, distinct from idle
  assert.notEqual(doc.link.href, idleHref);
});

test("bindFavicon: clean state when frame seeds with no open top-level tasks", () => {
  const frame = JSON.stringify({
    headEventId: 0,
    tasks: [{ shortId: "r1", title: "t", status: "done", parentShortId: null }],
    claims: [],
    blocks: [],
  });
  const doc = fakeDoc(frame);
  bindFavicon({ document: doc });
  doc.live.fire("connection", "connected");

  // 'clean' SVG carries a path (checkmark); 'idle' is just a circle.
  // Decode the data URL by removing the prefix and percent-decoding.
  const svg = decodeURIComponent(doc.link.href.replace(/^data:image\/svg\+xml,/, ""));
  assert.match(svg, /<path/);
});
