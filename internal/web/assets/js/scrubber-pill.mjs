/*
  Scrubber pill controller — toggles the dashboard between "live" and
  "scrubbing" modes. Live mode is the dashboard's default: footer shows
  a small "● Live" pill, the strip and history banner are hidden.
  Scrubbing mode reveals the strip (cursor, density, axis) and the
  amber history banner above the main content.

  This module owns the toggle and the visible state class. It does NOT
  own the cursor → frameAt fan-out (that's the next commit) or any
  per-view DOM updates. Drag handlers and density rendering also land
  in follow-on commits — what's here is the chrome on/off switch.

  Keyboard: Esc exits scrubbing as a quick-out. Click on either the
  pill itself or any [data-scrubber-return] button (in the strip and
  banner) returns to live.
*/

const PAGE_SCRUBBING_CLASS = "page--scrubbing";

function findPageRoot(doc) {
  // .page is the top-level scaffold. Tests may pass a fragment root
  // without it; defensively fall back to document.body.
  return doc.querySelector(".page") ?? doc.body;
}

export function enterScrubbing(doc = document) {
  const page = findPageRoot(doc);
  page.classList.add(PAGE_SCRUBBING_CLASS);
  toggleEls(doc, false);
  setPillLabel(doc, "Scrubbing");
}

export function exitScrubbing(doc = document) {
  const page = findPageRoot(doc);
  page.classList.remove(PAGE_SCRUBBING_CLASS);
  toggleEls(doc, true);
  setPillLabel(doc, "Live");
}

export function isScrubbing(doc = document) {
  return findPageRoot(doc).classList.contains(PAGE_SCRUBBING_CLASS);
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
}

if (typeof document !== "undefined") {
  // Run once DOM-ready. The bootstrap script itself loads as a module
  // (deferred), but we still want to be defensive in case markup is
  // injected later by a future enhancement.
  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", () => wireScrubberPill());
  } else {
    wireScrubberPill();
  }
}
