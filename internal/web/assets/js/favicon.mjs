// Dynamic favicon. Four states reflect the dashboard at a glance from
// the browser tab strip:
//
//   broken — SSE stream is offline or reconnecting (warn-orange)
//   clean  — connected, every top-level task is done/canceled (teal + check)
//   active — connected, at least one active claim (teal disc)
//   idle   — connected, work remains, nothing currently claimed (muted disc)
//
// Pure helpers (classifyFaviconState, applyEventToCounters,
// seedFromFrame, renderFaviconSVG, faviconDataURL) are exported and
// unit-tested in internal/web/jstest/favicon.test.mjs. bindFavicon
// wires the live document; tests use a fake document + synthetic
// <live-region> to exercise the controller without a browser.
//
// Colors are baked into the SVG as hex literals because data: URLs
// load in their own context and can't reach page CSS variables. If
// tokens.css ever moves these values, mirror them here.

const COLOR_PRIMARY = "#3cddc7";  // tokens.css --color-primary
const COLOR_WARN = "#e8865c";     // tokens.css --color-signal-warn
const COLOR_MUTED = "#5a6967";    // tokens.css --color-status-done

const CLAIM_INC_TYPES = new Set(["claimed"]);
const CLAIM_DEC_TYPES = new Set(["released", "done", "canceled", "claim_expired"]);

export function classifyFaviconState({ connection, activeClaims, openTopLevel }) {
  if (connection === "offline" || connection === "reconnecting") return "broken";
  if (openTopLevel === 0) return "clean";
  if (activeClaims > 0) return "active";
  return "idle";
}

export function seedFromFrame(payload) {
  const tasks = (payload && Array.isArray(payload.tasks)) ? payload.tasks : [];
  const claims = (payload && Array.isArray(payload.claims)) ? payload.claims : [];
  const topLevelStatus = new Map();
  let openTopLevel = 0;
  for (const t of tasks) {
    if (t.parentShortId == null) {
      topLevelStatus.set(t.shortId, t.status);
      if (t.status !== "done" && t.status !== "canceled") openTopLevel++;
    }
  }
  return { activeClaims: claims.length, openTopLevel, topLevelStatus };
}

export function applyEventToCounters(state, event) {
  if (!event || !event.type) return state;

  if (CLAIM_INC_TYPES.has(event.type)) {
    state.activeClaims = state.activeClaims + 1;
  } else if (CLAIM_DEC_TYPES.has(event.type)) {
    state.activeClaims = Math.max(0, state.activeClaims - 1);
  }

  const id = event.task_id;
  if (!id) return state;
  const detail = event.detail || {};

  if (event.type === "created") {
    if (!detail.parent_id) {
      state.topLevelStatus.set(id, "available");
      state.openTopLevel = state.openTopLevel + 1;
    }
  } else if (event.type === "done" || event.type === "canceled") {
    if (state.topLevelStatus.has(id)) {
      const prev = state.topLevelStatus.get(id);
      if (prev !== "done" && prev !== "canceled") {
        state.openTopLevel = Math.max(0, state.openTopLevel - 1);
      }
      state.topLevelStatus.set(id, event.type);
    }
  } else if (event.type === "reopened") {
    if (state.topLevelStatus.has(id)) {
      const prev = state.topLevelStatus.get(id);
      if (prev === "done" || prev === "canceled") {
        state.openTopLevel = state.openTopLevel + 1;
      }
      state.topLevelStatus.set(id, "available");
    }
  }

  return state;
}

export function renderFaviconSVG(state) {
  const s = (state === "broken" || state === "active" || state === "clean") ? state : "idle";
  const fill = s === "broken" ? COLOR_WARN
            : s === "active" ? COLOR_PRIMARY
            : s === "clean"  ? COLOR_PRIMARY
            : COLOR_MUTED;
  const head = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32">`;
  const disc = `<circle cx="16" cy="16" r="13" fill="${fill}"/>`;
  if (s === "clean") {
    // Checkmark sitting on the teal disc.
    return head + disc + `<path d="M9 16.5l4.5 4.5L23 11" fill="none" stroke="#0b1716" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"/></svg>`;
  }
  return head + disc + `</svg>`;
}

export function faviconDataURL(state) {
  return "data:image/svg+xml," + encodeURIComponent(renderFaviconSVG(state));
}

export function bindFavicon(opts) {
  const doc = opts.document;
  const link = doc.querySelector('link[rel~="icon"]');
  if (!link) return;
  const live = doc.querySelector("live-region");
  const island = doc.querySelector("#initial-frame");

  let payload = null;
  if (island && island.textContent) {
    try { payload = JSON.parse(island.textContent); } catch (_) { payload = null; }
  }

  const counters = seedFromFrame(payload);
  let connection = "connecting";
  let lastState = null;

  function refresh() {
    const next = classifyFaviconState({
      connection,
      activeClaims: counters.activeClaims,
      openTopLevel: counters.openTopLevel,
    });
    if (next === lastState) return;
    lastState = next;
    link.type = "image/svg+xml";
    link.href = faviconDataURL(next);
  }

  refresh();

  if (!live) return;
  live.addEventListener("connection", (e) => {
    connection = e.detail;
    refresh();
  });
  live.addEventListener("event", (e) => {
    applyEventToCounters(counters, e.detail);
    refresh();
  });
}

if (typeof document !== "undefined") {
  document.addEventListener("DOMContentLoaded", () => bindFavicon({ document }));
}
