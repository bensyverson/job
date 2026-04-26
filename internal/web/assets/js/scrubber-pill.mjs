/*
  Scrubber pill controller — toggles the dashboard between "live" and
  "scrubbing" modes and drives the cursor while scrubbing.

  Live mode is the default: footer shows the "● Live" pill, the strip
  and history banner are hidden, the dashboard's existing *-live.js
  modules handle SSE-driven updates as before. Entering scrubbing mode
  reveals the strip + banner, populates the density bars and cursor,
  and fans out a "jobs:scrubber-frame" CustomEvent on every cursor
  change so per-view modules can swap their DOM (per-view glue lands
  in follow-on commits).

  This module owns:
    - The toggle (click / Esc / "Return to live" buttons).
    - Density bar rendering on first activation.
    - Pointer-driven cursor drag with single-flight frame replays.
    - Banner text composition.

  It does NOT own per-view DOM updates. Listeners attach to the
  document via 'jobs:scrubber-frame' / 'jobs:scrubber-live' events.
*/

import {
  xToEventId,
  computeDensityBars,
  formatHistoryBannerText,
} from "./scrubber-cursor.mjs";

const PAGE_SCRUBBING_CLASS = "page--scrubbing";

// Module-level state. Populated lazily on first scrubbing entry; the
// dashboard often loads in live mode and never enters the scrubber,
// so we don't pay the events-fetch cost up-front.
let events = [];
let nowMs = 0;
let buf = null;
let initialized = false;

// Single-flight queue for cursor-driven replays. Pointermove can
// fire 60+ times per second; ReplayBuffer.frameAt is async. Without
// gating, promises pile up and DOM updates land out of order.
let inFlightX = null;
let queuedX = null;

function findPageRoot(doc) {
  return doc.querySelector(".page") ?? doc.body;
}

function toggleEls(doc, hide) {
  for (const el of doc.querySelectorAll("[data-scrubber-strip], [data-history-banner]")) {
    el.hidden = hide;
  }
}

function setPillLabel(doc, text) {
  const label = doc.querySelector("[data-scrubber-pill-label]");
  if (label) label.textContent = text;
}

function setCursor(doc, xFrac) {
  const strip = doc.querySelector("[data-scrubber-strip]");
  if (strip) strip.style.setProperty("--x", `${(xFrac * 100).toFixed(1)}%`);
}

function renderDensityBars(doc) {
  const container = doc.querySelector("[data-scrubber-bars]");
  if (!container) return;
  const bars = computeDensityBars(events, nowMs);
  // Idempotent: only re-render if bar count changed (it won't, but a
  // future opt-in to re-bin on viewport resize could).
  if (container.children.length === bars.length) {
    for (let i = 0; i < bars.length; i++) {
      container.children[i].style.setProperty("--h", `${bars[i]}%`);
    }
    return;
  }
  container.replaceChildren();
  for (const h of bars) {
    const span = doc.createElement("span");
    span.className = "c-scrubber-strip__bar";
    span.style.setProperty("--h", `${h}%`);
    container.appendChild(span);
  }
}

function updateBanner(doc, event) {
  const text = event ? formatHistoryBannerText(event, nowMs) : "—";
  for (const el of doc.querySelectorAll("[data-scrubber-at], [data-history-banner-at]")) {
    el.textContent = text;
  }
}

async function applyCursor(doc, xFrac) {
  if (events.length === 0 || !buf) return;
  const eventId = xToEventId(xFrac, events, nowMs);
  if (eventId === 0) return;
  const event = events.find((e) => e.id === eventId);
  const frame = await buf.frameAt(eventId);
  updateBanner(doc, event);
  doc.dispatchEvent(
    new CustomEvent("jobs:scrubber-frame", { detail: { frame, event } }),
  );
}

// setCursorByX runs at most one applyCursor at a time, queuing the
// most recent target while one is in flight. When the in-flight call
// resolves, we apply the queued x (which may itself be replaced
// during that resolution) and loop until queuedX drains.
async function setCursorByX(doc, xFrac) {
  setCursor(doc, xFrac);
  if (inFlightX !== null) {
    queuedX = xFrac;
    return;
  }
  inFlightX = xFrac;
  try {
    await applyCursor(doc, xFrac);
    while (queuedX !== null) {
      const next = queuedX;
      queuedX = null;
      inFlightX = next;
      await applyCursor(doc, next);
    }
  } finally {
    inFlightX = null;
  }
}

async function ensureInitialized(doc) {
  if (initialized) return;
  initialized = true;
  buf = (typeof window !== "undefined" ? window.JobsScrubber : null) ?? null;
  if (!buf) return;
  nowMs = Date.now();
  try {
    events = await buf.range(0, buf.headFrame.eventId);
  } catch {
    // If event fetch fails, density bars stay empty; the cursor still
    // toggles visually but frameAt calls are no-ops. The user gets a
    // degraded scrubber rather than a broken page.
    events = [];
  }
  renderDensityBars(doc);
}

export async function enterScrubbing(doc = document) {
  await ensureInitialized(doc);
  const page = findPageRoot(doc);
  page.classList.add(PAGE_SCRUBBING_CLASS);
  toggleEls(doc, false);
  setPillLabel(doc, "Scrubbing");
  // Default cursor at "now" — entering scrubbing without a drag
  // means "I want to see what just happened." A subsequent drag
  // moves it back in time.
  setCursor(doc, 1);
  if (events.length > 0) {
    updateBanner(doc, events[events.length - 1]);
  }
}

export function exitScrubbing(doc = document) {
  const page = findPageRoot(doc);
  page.classList.remove(PAGE_SCRUBBING_CLASS);
  toggleEls(doc, true);
  setPillLabel(doc, "Live");
  setCursor(doc, 1);
  doc.dispatchEvent(new CustomEvent("jobs:scrubber-live"));
}

export function isScrubbing(doc = document) {
  return findPageRoot(doc).classList.contains(PAGE_SCRUBBING_CLASS);
}

function wireDrag(doc) {
  const track = doc.querySelector("[data-scrubber-track]");
  if (!track) return;
  let dragging = false;
  function xFrom(ev) {
    const rect = track.getBoundingClientRect();
    if (rect.width === 0) return 0;
    const raw = (ev.clientX - rect.left) / rect.width;
    return Math.max(0, Math.min(1, raw));
  }
  track.addEventListener("pointerdown", (ev) => {
    dragging = true;
    if (track.setPointerCapture) track.setPointerCapture(ev.pointerId);
    setCursorByX(doc, xFrom(ev));
  });
  track.addEventListener("pointermove", (ev) => {
    if (!dragging) return;
    setCursorByX(doc, xFrom(ev));
  });
  function stopDrag(ev) {
    dragging = false;
    if (track.releasePointerCapture) {
      try {
        track.releasePointerCapture(ev.pointerId);
      } catch {
        // Already released; ignore.
      }
    }
  }
  track.addEventListener("pointerup", stopDrag);
  track.addEventListener("pointercancel", stopDrag);
}

export function wireScrubberPill(doc = document) {
  const pill = doc.querySelector("[data-scrubber-toggle]");
  if (pill) {
    pill.addEventListener("click", () => {
      if (isScrubbing(doc)) exitScrubbing(doc);
      else enterScrubbing(doc);
    });
  }
  for (const btn of doc.querySelectorAll("[data-scrubber-return]")) {
    btn.addEventListener("click", () => exitScrubbing(doc));
  }
  doc.addEventListener("keydown", (e) => {
    if (e.key === "Escape" && isScrubbing(doc)) exitScrubbing(doc);
  });
  wireDrag(doc);
}

if (typeof document !== "undefined") {
  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", () => wireScrubberPill());
  } else {
    wireScrubberPill();
  }
}
