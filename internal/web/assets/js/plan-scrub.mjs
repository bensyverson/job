/*
  Plan-view scrubber driver.

  When the scrubber pill dispatches 'jobs:scrubber-frame' on the
  document, this module rebuilds <main .c-section[aria-label="Plan"]>
  (and the filter bar above it) from the in-memory Frame, mirroring
  what plan.go would render server-side at the cursor's event id.
  Live mode is restored by 'jobs:scrubber-live': re-fetch the live
  /plan URL and swap the freshly-rendered section back in. SSE is
  paused while scrubbing (the page carries the page--scrubbing class),
  so the explicit re-fetch is the simplest way to re-sync.

  The pure helpers (parseFiltersFromSearch, buildShowTabs,
  buildPlanLabelChips, composePlanFilterBarShape) are exported for
  tests; the auto-init at the bottom only fires in a browser.

  Filter parameters (?label=, ?show=) are honored both server-side
  and here so a user who navigates a label-filtered Plan and then
  scrubs sees the same subset under the cursor. ?at= is the scrubber's
  own param and is ignored by this module.
*/

import {
  buildForestFromFrame,
  filterRootsByShow,
  filterForestByLabels,
  pickStripLabels,
  buildPlanNodes,
  planURL,
  toggleLabel,
} from "./plan-scrub-build.mjs";
import { renderPlanSection, renderFilterBar } from "./plan-scrub-render.mjs";

const STRIP_LIMIT = 5;

// parseFiltersFromSearch reads ?label= and ?show= out of a query
// string. Mirrors the Go-side parseLabelParam / parseShowParam: comma-
// split, trim, dedupe, sort; unknown ?show values fall back to the
// default ("active"). ?at= belongs to the scrubber and is ignored.
export function parseFiltersFromSearch(search) {
  const params = new URLSearchParams(search ?? "");
  const rawLabel = params.get("label") ?? "";
  const seen = new Set();
  for (const part of rawLabel.split(",")) {
    const s = part.trim();
    if (s !== "") seen.add(s);
  }
  const selected = [...seen].sort();
  const rawShow = (params.get("show") ?? "").trim();
  const show =
    rawShow === "archived" || rawShow === "all" ? rawShow : "active";
  return { selected, show };
}

// buildShowTabs emits the Active/Archived/All tabs preserving the
// current label selection. Same shape as plan.go's buildShowTabs.
export function buildShowTabs(selected, show) {
  return [
    { label: "Active", url: planURL(selected, "active"), active: show === "active" },
    { label: "Archived", url: planURL(selected, "archived"), active: show === "archived" },
    { label: "All", url: planURL(selected, "all"), active: show === "all" },
  ];
}

// buildPlanLabelChips turns a strip-name list into the renderFilterBar
// shape. Each chip's URL toggles that label in/out of the selection;
// active flag is true when the label is currently selected.
export function buildPlanLabelChips(stripNames, selected, show) {
  const sel = new Set(selected);
  return stripNames.map((name) => ({
    name,
    url: planURL(toggleLabel(selected, name), show),
    active: sel.has(name),
  }));
}

// composePlanFilterBarShape walks the frame to derive the filter-bar
// input renderFilterBar consumes: the show tabs, the label strip, the
// "any" pill URL/active flag. Centralizes the chrome computation so
// the driver can re-emit it on every frame swap.
export function composePlanFilterBarShape(frame, { selected, show }) {
  // Strip labels are computed against the *unfiltered* forest so they
  // stay stable across label clicks (matches plan.go's two-pass shape).
  const allRoots = buildForestFromFrame(frame);
  const stripNames = pickStripLabels(allRoots, selected, show, STRIP_LIMIT);
  return {
    showTabs: buildShowTabs(selected, show),
    labels: buildPlanLabelChips(stripNames, selected, show),
    allURL: planURL([], show),
    allActive: selected.length === 0,
  };
}

// composePlanSectionHTML returns the HTML string for the Plan
// <section> at the given frame and filter set. Driver swaps this in
// via DOMParser.
function composePlanSectionHTML(frame, { selected, show }, nowSec) {
  const allRoots = buildForestFromFrame(frame);
  let roots = filterRootsByShow(allRoots, show);
  roots = filterForestByLabels(roots, selected);
  const planNodes = buildPlanNodes(roots, frame, nowSec, { selected, show });
  return renderPlanSection(planNodes);
}

// --- DOM driver ---
//
// Browser-only. Listens on the document for the scrubber's events,
// runs DOMParser on a freshly-rendered section/filter-bar string, and
// swaps the live nodes. After the swap, runs the same idempotent
// re-hydration plan-live.js does (color paint, plan-collapse).

function findPlanSection(doc) {
  return doc.querySelector("main .c-section[aria-label='Plan']");
}

function findFilterBarHost(doc) {
  // The filter bar lives just above the Plan section in the SSR
  // output: the row of tabs + the c-filter-bar section. The simplest
  // swap target is the c-filter-bar element; the tabs row is a sibling
  // and we replace both as a pair.
  return doc.querySelector("main section.c-filter-bar");
}

function rehydrate(doc) {
  if (window.JobsColors) {
    if (typeof window.JobsColors.paint === "function") {
      window.JobsColors.paint(doc);
    }
    if (typeof window.JobsColors.paintProgress === "function") {
      window.JobsColors.paintProgress(doc);
    }
  }
  if (
    window.JobsPlanCollapse &&
    typeof window.JobsPlanCollapse.applyStored === "function"
  ) {
    window.JobsPlanCollapse.applyStored();
  }
}

function swapHTMLInto(html, oldEl) {
  // Wrap fragments in a host element since DOMParser needs a full
  // document; we read children back out and replaceWith preserves
  // surrounding markup.
  const doc = new DOMParser().parseFromString(`<body>${html}</body>`, "text/html");
  const fresh = Array.from(doc.body.children);
  if (fresh.length === 0) return;
  const ref = oldEl;
  ref.replaceWith(...fresh);
}

function applyFrameToDOM(frame, event) {
  const doc = document;
  const section = findPlanSection(doc);
  if (!section) return;
  const filters = parseFiltersFromSearch(window.location.search);
  const nowSec = event?.created_at ?? Math.floor(Date.now() / 1000);

  // Replace the filter bar (tabs + c-filter-bar) and the Plan section.
  const filterHost = findFilterBarHost(doc);
  if (filterHost) {
    const tabsRow = filterHost.previousElementSibling;
    const barShape = composePlanFilterBarShape(frame, filters);
    const html = renderFilterBar(barShape);
    // Swap both the tabs row and the filter bar in one go.
    if (tabsRow && tabsRow.classList.contains("row")) {
      tabsRow.remove();
    }
    swapHTMLInto(html, filterHost);
  }

  const sectionHTML = composePlanSectionHTML(frame, filters, nowSec);
  swapHTMLInto(sectionHTML, findPlanSection(doc));
  rehydrate(doc);
}

async function refetchLive() {
  const doc = document;
  const section = findPlanSection(doc);
  if (!section) return;
  try {
    const res = await fetch(window.location.href, {
      headers: { Accept: "text/html" },
      credentials: "same-origin",
    });
    if (!res.ok) return;
    const html = await res.text();
    const fresh = new DOMParser().parseFromString(html, "text/html");
    const freshSection = fresh.querySelector("main .c-section[aria-label='Plan']");
    if (freshSection) findPlanSection(doc).replaceWith(freshSection);
    const freshBar = fresh.querySelector("main section.c-filter-bar");
    const oldBar = doc.querySelector("main section.c-filter-bar");
    if (freshBar && oldBar) oldBar.replaceWith(freshBar);
    rehydrate(doc);
  } catch (_) {
    // Network blip; the next live event triggers another refresh
    // through the existing plan-live.js path.
  }
}

if (typeof document !== "undefined") {
  function init() {
    if (!findPlanSection(document)) return;
    document.addEventListener("jobs:scrubber-frame", (e) => {
      const buf = window.JobsScrubber;
      if (!buf) return;
      // The CustomEvent's detail.frame is the cached frame at the
      // cursor; detail.event is the underlying server event so we can
      // freeze RelTime against its created_at.
      applyFrameToDOM(e.detail.frame, e.detail.event);
    });
    document.addEventListener("jobs:scrubber-live", () => {
      refetchLive();
    });
  }
  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init);
  } else {
    init();
  }
}
