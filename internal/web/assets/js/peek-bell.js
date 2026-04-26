/*
  Per-tab notification bell. Toggles whether THIS tab fires a browser
  notification when the bell's task transitions to "done".

  The subscription set lives in sessionStorage so it survives a peek-
  open within the tab but doesn't leak across tabs or windows. Two
  open tabs of the same task can independently subscribe — each owns
  its own bell visual + notifications.

  Wiring:

    - Click on any [data-task-bell] flips that task's subscription.
      First subscribe in a tab requests Notification.permission if
      not yet granted; if the user denies, the visual rolls back.
    - Bells reflect current state via aria-pressed + the
      c-bell--active CSS modifier. After a peek opens (fragment
      mounts), the module repaints any newly-visible bells.
    - When the live-region delivers a "done" event for a subscribed
      task, fire `new Notification(...)` and unsubscribe (one-shot;
      the user can re-subscribe to await the next state change, but
      a completed task doesn't keep paging them).

  No-JS fallback: the bell button still renders, just doesn't toggle.
*/

(function () {
  "use strict";

  const STORAGE_KEY = "jobs.peek.bell.subscriptions";

  document.addEventListener("DOMContentLoaded", init);

  function init() {
    document.addEventListener("click", onClick);

    const live = document.querySelector("live-region");
    if (live) {
      live.addEventListener("event", onLiveEvent);
    }

    // Bells in the SSR-rendered DOM (task page header) need their
    // initial visual sync. Bells inside a peek fragment are mounted
    // later — observe the <peek-sheet> host so we can repaint when
    // its content swaps in.
    syncAllBells();
    const peek = document.querySelector("peek-sheet");
    if (peek && typeof MutationObserver === "function") {
      new MutationObserver(syncAllBells).observe(peek, { childList: true, subtree: true });
    }
  }

  function onClick(ev) {
    if (ev.defaultPrevented || ev.button !== 0) return;
    const bell = ev.target && ev.target.closest && ev.target.closest("[data-task-bell]");
    if (!bell) return;
    ev.preventDefault();
    toggle(bell);
  }

  async function toggle(bell) {
    const id = bell.getAttribute("data-task-bell");
    if (!id) return;

    if (isSubscribed(id)) {
      unsubscribe(id);
      paintBell(bell, false);
      return;
    }

    // Subscribing: ensure permission first. If the user denies, do
    // not record the subscription and leave the bell off.
    const ok = await ensurePermission();
    if (!ok) {
      paintBell(bell, false);
      return;
    }
    subscribe(id);
    paintBell(bell, true);
  }

  function onLiveEvent(ev) {
    const data = ev.detail;
    if (!data || data.event_type !== "done") return;
    const id = data.task_id;
    if (!id || !isSubscribed(id)) return;

    fireNotification(data);
    // One-shot. Drop the subscription so we don't re-fire on a
    // future done (e.g. a re-opened task that closes again).
    unsubscribe(id);
    syncAllBells();
  }

  function fireNotification(e) {
    if (typeof Notification === "undefined" || Notification.permission !== "granted") {
      return;
    }
    const title = "Done · " + (e.task_title || e.task_id || "Task");
    const body = e.actor ? "completed by " + e.actor : "Task completed";
    try {
      const n = new Notification(title, {
        body: body,
        tag: "jobs-task-" + e.task_id,
        // Click-through to the full task page rather than re-opening
        // the peek sheet — at this point the user is being told
        // about something they're not currently looking at.
      });
      n.onclick = function () {
        window.focus();
        if (e.task_id) {
          window.location.href = "/tasks/" + encodeURIComponent(e.task_id);
        }
        n.close();
      };
    } catch (_) {
      // Old browsers may throw on the constructor without secure
      // context. Silent — the bell already gave the user the only
      // promise we can keep.
    }
  }

  // --- subscription set ---------------------------------------------

  function read() {
    try {
      const raw = window.sessionStorage.getItem(STORAGE_KEY);
      if (!raw) return new Set();
      const arr = JSON.parse(raw);
      return Array.isArray(arr) ? new Set(arr) : new Set();
    } catch (_) {
      return new Set();
    }
  }

  function write(set) {
    try {
      window.sessionStorage.setItem(STORAGE_KEY, JSON.stringify(Array.from(set)));
    } catch (_) {
      // sessionStorage may be unavailable in private modes; fall
      // back to a soft no-op so the bell still toggles visually.
    }
  }

  function isSubscribed(id) { return read().has(id); }
  function subscribe(id) { const s = read(); s.add(id); write(s); }
  function unsubscribe(id) { const s = read(); s.delete(id); write(s); }

  // --- permission ---------------------------------------------------

  async function ensurePermission() {
    if (typeof Notification === "undefined") return false;
    if (Notification.permission === "granted") return true;
    if (Notification.permission === "denied") return false;
    try {
      const result = await Notification.requestPermission();
      return result === "granted";
    } catch (_) {
      return false;
    }
  }

  // --- visual sync --------------------------------------------------

  function syncAllBells() {
    const bells = document.querySelectorAll("[data-task-bell]");
    bells.forEach((bell) => {
      const id = bell.getAttribute("data-task-bell");
      paintBell(bell, id ? isSubscribed(id) : false);
    });
  }

  function paintBell(bell, on) {
    bell.setAttribute("aria-pressed", on ? "true" : "false");
    bell.classList.toggle("c-bell--active", !!on);
  }
})();
