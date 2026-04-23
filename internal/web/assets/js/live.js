/*
  Live updates — thin WebComponent wrapper around EventSource.
  Mount as <live-region src="/events?…"> (default src: /events). Views
  subscribe to two custom events on the element:

    'event'      detail = parsed event object from the server
    'connection' detail = 'connecting' | 'connected' | 'reconnecting'

  What this module owns:

    - EventSource lifecycle (open on connect, close on disconnect).
    - last-event-id persistence across reloads via localStorage so the
      dashboard resumes from where it stopped rather than missing the
      window between page-load-start and SSE-open.
    - Heartbeat rendering into [data-heartbeat] in the footer: pulsing
      dot + "last event Ns ago" / "reconnecting…" / "offline".

  What this module does NOT own:

    - DOM updates for any specific view. Views listen for the 'event'
      CustomEvent and update their own rows/columns. The module is
      intentionally ignorant of Log / Home / Actors structure.

  EventSource already handles reconnect with exponential backoff and
  resends Last-Event-ID automatically, so we get most of the reconnect
  behavior for free. Our localStorage persistence handles the separate
  page-reload case.
*/

(function () {
  "use strict";

  const STORAGE_KEY = "jobs.live.lastEventId";
  const HEARTBEAT_REFRESH_MS = 1000;

  // Event types we know about. Listening for 'message' is not enough
  // because SSE frames with `event: <name>` dispatch only to the
  // matching listener. Keep this list in sync with the server's
  // event_type vocabulary.
  const KNOWN_EVENT_TYPES = [
    "created",
    "claimed",
    "released",
    "done",
    "reopened",
    "canceled",
    "noted",
    "blocked",
    "unblocked",
    "labeled",
    "unlabeled",
    "moved",
    "edited",
    "heartbeat",
  ];

  function saveLastID(id) {
    try {
      localStorage.setItem(STORAGE_KEY, String(id));
    } catch (_) {
      // Private browsing / storage quotas — safe to ignore; reconnect
      // still works within a single page life via EventSource's own
      // Last-Event-ID tracking.
    }
  }

  function loadLastID() {
    try {
      const v = localStorage.getItem(STORAGE_KEY);
      if (!v) return null;
      const n = parseInt(v, 10);
      return Number.isFinite(n) && n > 0 ? n : null;
    } catch (_) {
      return null;
    }
  }

  class LiveRegion extends HTMLElement {
    constructor() {
      super();
      this.es = null;
      this.lastEventAt = 0; // wall-clock ms of most recent event
      this.connectionState = "connecting";
    }

    connectedCallback() {
      const raw = this.getAttribute("src") || "/events";
      const url = new URL(raw, window.location.origin);
      // Resume from the last id we saw in any prior session. The
      // server's backfill will replay everything since — modulo limit.
      const resume = loadLastID();
      if (resume && !url.searchParams.has("since")) {
        url.searchParams.set("since", String(resume));
      }
      this.openStream(url.toString());
      this.startHeartbeatTick();

      // navigator.onLine distinguishes "your laptop's wifi dropped"
      // from "server is briefly unreachable but network is fine" —
      // both surface as EventSource errors otherwise.
      this.onOnline = () => {
        if (this.connectionState === "offline") this.setConnection("reconnecting");
      };
      this.onOffline = () => this.setConnection("offline");
      window.addEventListener("online", this.onOnline);
      window.addEventListener("offline", this.onOffline);
      if (typeof navigator !== "undefined" && navigator.onLine === false) {
        this.setConnection("offline");
      }
    }

    disconnectedCallback() {
      if (this.es) this.es.close();
      this.es = null;
      if (this.tickHandle) clearInterval(this.tickHandle);
      if (this.onOnline) window.removeEventListener("online", this.onOnline);
      if (this.onOffline) window.removeEventListener("offline", this.onOffline);
    }

    openStream(url) {
      this.es = new EventSource(url);

      this.es.addEventListener("open", () => {
        this.setConnection("connected");
      });
      this.es.addEventListener("error", () => {
        // EventSource will retry automatically; we surface the state
        // so the UI can reflect it without killing the stream.
        this.setConnection("reconnecting");
      });

      for (const type of KNOWN_EVENT_TYPES) {
        this.es.addEventListener(type, (ev) => this.handleFrame(ev));
      }
      // Catch-all for any event type we didn't pre-register. SSE
      // dispatch to 'message' only fires when no event: field is
      // present, so this picks up server frames that omit it.
      this.es.addEventListener("message", (ev) => this.handleFrame(ev));
    }

    handleFrame(ev) {
      let data = null;
      try {
        data = JSON.parse(ev.data);
      } catch (_) {
        return;
      }
      if (data && typeof data.id === "number") {
        saveLastID(data.id);
      }
      this.lastEventAt = Date.now();
      this.dispatchEvent(new CustomEvent("event", { detail: data }));
    }

    setConnection(state) {
      if (state === this.connectionState) return;
      this.connectionState = state;
      this.dispatchEvent(new CustomEvent("connection", { detail: state }));
      renderHeartbeat(state, this.lastEventAt);
    }

    startHeartbeatTick() {
      renderHeartbeat(this.connectionState, this.lastEventAt);
      this.tickHandle = setInterval(() => {
        renderHeartbeat(this.connectionState, this.lastEventAt);
      }, HEARTBEAT_REFRESH_MS);
    }
  }

  // renderHeartbeat updates both footer slots.
  //   [data-heartbeat] gets the "last event Ns ago" rhythm — it's the
  //                    "is work still happening?" signal.
  //   [data-connection] gets the SSE connection state word —
  //                    "Connected" / "Reconnecting" / "Offline" — it's
  //                    the "is the stream alive?" signal. The two
  //                    disagree during an active outage, which is
  //                    exactly when a viewer most wants to tell them
  //                    apart.
  function renderHeartbeat(state, lastEventAt) {
    const hb = document.querySelector("[data-heartbeat]");
    if (hb) {
      const label = hb.querySelector("span:last-child") || hb;
      if (state === "connected") {
        label.textContent = lastEventAt
          ? "last event " + agoLabel(Date.now() - lastEventAt)
          : "waiting for first event";
      } else if (state === "reconnecting") {
        label.textContent = "reconnecting…";
      } else if (state === "offline") {
        label.textContent = "offline";
      } else {
        label.textContent = "connecting…";
      }
      hb.dataset.state = state;
    }

    const conn = document.querySelector("[data-connection]");
    if (conn) {
      const text =
        state === "connected" ? "Connected" :
        state === "reconnecting" ? "Reconnecting" :
        state === "offline" ? "Offline" :
        "Connecting";
      conn.textContent = text;
      conn.dataset.state = state;
    }
  }

  function agoLabel(ms) {
    const s = Math.max(0, Math.round(ms / 1000));
    if (s < 60) return s + "s ago";
    const m = Math.floor(s / 60);
    if (m < 60) return m + "m ago";
    const h = Math.floor(m / 60);
    return h + "h ago";
  }

  if (!customElements.get("live-region")) {
    customElements.define("live-region", LiveRegion);
  }

  // Expose for test scripts / console poking.
  window.JobsLive = { KNOWN_EVENT_TYPES: KNOWN_EVENT_TYPES };
})();
