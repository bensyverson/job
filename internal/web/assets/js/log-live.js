/*
  Log view live-tail. Subscribes to the <live-region>'s 'event'
  custom event and prepends a new .c-log-row to the .c-log list for
  each incoming event. Server-side already filters the SSE stream to
  match the page's filter state (see LogPageData.EventsURL), so this
  module doesn't need to re-check filters — it just renders.

  Self-guarded: if the page has no .c-log, the module is a no-op.
  That way we can load it from the shared layout without per-page
  wiring.
*/

(function () {
  "use strict";

  document.addEventListener("DOMContentLoaded", init);

  function init() {
    const list = document.querySelector(".c-log");
    if (!list) return;

    const live = document.querySelector("live-region");
    if (!live) return;

    const empty = list.querySelector(".c-log-row--empty");

    // Dedup set: every event id already present in the DOM (server
    // SSR plus any rows we've prepended). Backfill/SSR overlap
    // happens when the page loads with events already rendered and
    // the SSE stream replays them from localStorage-resumed ?since=;
    // without this set we'd duplicate every overlapping row.
    const seen = new Set();
    list.querySelectorAll("[data-event-id]").forEach((el) => {
      seen.add(el.getAttribute("data-event-id"));
    });

    live.addEventListener("event", (ev) => {
      const data = ev.detail;
      if (!data || data.id == null) return;

      const idStr = String(data.id);
      if (seen.has(idStr)) return;
      seen.add(idStr);

      if (empty && empty.parentElement) empty.remove();

      const row = renderRow(data);
      if (!row) return;
      list.prepend(row);

      if (window.JobsColors && typeof window.JobsColors.paint === "function") {
        window.JobsColors.paint(row);
      }

      // Trim the live strip after it has grown past a reasonable
      // cap. 500 matches the server-side JSON limit. Keep the dedup
      // set trimmed in parallel so long-running tabs don't grow it
      // without bound.
      const MAX_ROWS = 500;
      while (list.childElementCount > MAX_ROWS) {
        const dropped = list.lastElementChild;
        const droppedID = dropped && dropped.getAttribute("data-event-id");
        list.removeChild(dropped);
        if (droppedID) seen.delete(droppedID);
      }
    });
  }

  // renderRow mirrors the server-side c-log-row markup in log.html.tmpl.
  // Keep the two in sync when the server template changes.
  function renderRow(e) {
    const row = document.createElement("div");
    row.className = "c-log-row c-log-row--" + safeClass(e.event_type) + " c-log-row--new";
    row.setAttribute("role", "listitem");
    if (e.id != null) row.setAttribute("data-event-id", String(e.id));

    const time = document.createElement("time");
    time.className = "c-log-row__time";
    time.dateTime = e.created_at || "";
    time.textContent = "just now";
    row.appendChild(time);

    const actorLink = document.createElement("a");
    actorLink.className = "c-log-row__actor";
    actorLink.href = "/actors/" + encodeURIComponent(e.actor || "");
    actorLink.setAttribute("data-actor", e.actor || "");
    const avatar = document.createElement("span");
    avatar.className = "c-avatar c-avatar-sm";
    avatar.setAttribute("data-actor", e.actor || "");
    actorLink.appendChild(avatar);
    const actorName = document.createElement("span");
    actorName.textContent = e.actor || "";
    actorLink.appendChild(actorName);
    row.appendChild(actorLink);

    const verb = document.createElement("span");
    verb.className = "c-log-row__verb c-log-row__verb--" + safeClass(e.event_type);
    verb.textContent = e.event_type || "";
    row.appendChild(verb);

    const id = document.createElement("span");
    id.className = "c-id-pill";
    id.textContent = e.task_id || "";
    row.appendChild(id);

    const detail = document.createElement("span");
    detail.className = "c-log-row__detail";
    if (e.task_title) {
      const title = document.createElement("span");
      title.className = "c-log-row__title";
      title.textContent = e.task_title;
      detail.appendChild(title);
    }
    const note = extractNote(e);
    if (note) {
      const noteEl = document.createElement("span");
      noteEl.className = "c-log-row__note";
      noteEl.textContent = note;
      detail.appendChild(noteEl);
    }
    row.appendChild(detail);

    const link = document.createElement("a");
    link.className = "c-row-link";
    link.href = "/tasks/" + encodeURIComponent(e.task_id || "");
    link.setAttribute("aria-label", "Open task " + (e.task_id || ""));
    row.appendChild(link);

    return row;
  }

  // extractNote mirrors the server-side notePreviewFromDetail logic:
  // done/canceled carry "note", noted carries "text", others render
  // without a note body.
  function extractNote(e) {
    if (!e.detail) return "";
    let parsed;
    try { parsed = JSON.parse(e.detail); } catch (_) { return ""; }
    if (!parsed || typeof parsed !== "object") return "";
    const key = (e.event_type === "noted") ? "text" : (e.event_type === "done" || e.event_type === "canceled") ? "note" : null;
    if (!key) return "";
    const v = parsed[key];
    if (typeof v !== "string") return "";
    const trimmed = v.trim();
    return trimmed.length > 160 ? trimmed.slice(0, 160) + "…" : trimmed;
  }

  // safeClass defensively sanitizes an event_type before inlining it
  // into a class token. The server currently only produces lowercase
  // ASCII strings here, but we don't want a malformed detail to ever
  // become a CSS-class injection vector.
  function safeClass(s) {
    return String(s || "").replace(/[^a-zA-Z0-9_-]/g, "");
  }
})();
