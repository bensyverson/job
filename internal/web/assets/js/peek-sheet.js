/*
  <peek-sheet> custom element. Owns the slide-in side panel that
  shows a task's peek fragment without leaving the current page.

  Public API:

    el.open(shortID)   fetch /tasks/<id>/peek, mount the fragment,
                       slide in. Pushes ?preview=<id> onto the URL.
    el.close()         slide out, drop the fragment from the DOM,
                       remove ?preview from the URL.

  Module-level convenience:

    window.JobsPeek.open(shortID)
    window.JobsPeek.close()

  Behaviors:

    - Auto-open on connectedCallback if the URL carries ?preview=<id>.
    - Escape key closes (only when the sheet is open).
    - Clicking the underlay closes.
    - Clicking [data-peek-close] inside the fragment closes.
    - popstate re-syncs to whatever ?preview= is now in the URL,
      so the browser's back/forward buttons drive the sheet state.

  Light DOM is intentional. Shadow DOM would isolate the host's CSS,
  forcing us to either inline tokens or duplicate component styles —
  neither is worth it for what's effectively a styled panel.
*/

(function () {
  "use strict";

  const STATE_IDLE = "";
  const STATE_OPEN = "open";
  const STATE_CLOSING = "closing";

  class PeekSheet extends HTMLElement {
    constructor() {
      super();
      this._currentID = null;
      this._abort = null;
      this._onKey = this._onKey.bind(this);
      this._onPopState = this._onPopState.bind(this);
      this._onClick = this._onClick.bind(this);
    }

    connectedCallback() {
      // Idle visual state until first open.
      if (!this.dataset.state) this.dataset.state = STATE_IDLE;
      document.addEventListener("keydown", this._onKey);
      window.addEventListener("popstate", this._onPopState);
      this.addEventListener("click", this._onClick);

      const params = new URLSearchParams(window.location.search);
      const preview = params.get("preview");
      if (preview) {
        // Fire-and-forget; we don't want to block render on a fetch.
        this.open(preview, { skipPushState: true });
      }
    }

    disconnectedCallback() {
      document.removeEventListener("keydown", this._onKey);
      window.removeEventListener("popstate", this._onPopState);
      this.removeEventListener("click", this._onClick);
      if (this._abort) this._abort.abort();
    }

    async open(shortID, opts) {
      opts = opts || {};
      if (!shortID) return;
      if (this._currentID === shortID && this.dataset.state === STATE_OPEN) return;
      this._currentID = shortID;

      // Cancel any in-flight fetch — switching tasks shouldn't show
      // stale content if a slow response races a fast one.
      if (this._abort) this._abort.abort();
      this._abort = new AbortController();

      let html;
      try {
        const res = await fetch("/tasks/" + encodeURIComponent(shortID) + "/peek", {
          signal: this._abort.signal,
          headers: { "Accept": "text/html" },
        });
        html = await res.text();
        // 404 / 500 still return fragment-shaped error markup, so
        // we don't need to branch on res.ok — the fragment IS the
        // status. The user sees the templated "Not available" sheet.
      } catch (err) {
        if (err.name === "AbortError") return;
        // Network failure: render a minimal inline error.
        html = `<aside class="c-peek-sheet c-peek-sheet--error" role="complementary">
          <div class="c-peek-sheet__header">
            <h2 class="c-peek-sheet__title">Couldn't load</h2>
            <button class="c-peek-sheet__close" data-peek-close aria-label="Close peek">×</button>
          </div>
          <p class="t-muted t-body-sm">Check your connection and try again.</p>
        </aside>`;
      }

      // Mount the fragment plus an underlay, then transition.
      this.innerHTML =
        '<div class="c-peek-underlay" data-peek-underlay aria-hidden="true"></div>' +
        html;

      // Force a reflow before flipping to "open" so the transition
      // starts from the off-screen state rather than skipping
      // straight to the end.
      this.dataset.state = STATE_IDLE;
      // eslint-disable-next-line no-unused-expressions
      this.offsetWidth;
      this.dataset.state = STATE_OPEN;

      // Repaint avatars / labels in the freshly-mounted fragment.
      if (window.JobsColors && typeof window.JobsColors.paint === "function") {
        window.JobsColors.paint(this);
      }

      if (!opts.skipPushState) {
        const url = new URL(window.location.href);
        url.searchParams.set("preview", shortID);
        history.pushState({ peek: shortID }, "", url);
      }

      // Move focus to the close button so keyboard users can dismiss
      // immediately. Falls back silently if the fragment somehow
      // shipped without one (e.g. an old cached error response).
      const closer = this.querySelector("[data-peek-close]");
      if (closer && typeof closer.focus === "function") {
        closer.focus({ preventScroll: true });
      }
    }

    close(opts) {
      opts = opts || {};
      if (this.dataset.state !== STATE_OPEN) return;
      this._currentID = null;
      this.dataset.state = STATE_CLOSING;

      // Wait for the slide-out transition to finish, then drop the
      // fragment from the DOM. Falls back to a fixed timeout when
      // transitionend doesn't fire (reduced-motion, detached node).
      const sheet = this.querySelector(".c-peek-sheet");
      const finish = () => {
        if (this.dataset.state !== STATE_CLOSING) return;
        this.dataset.state = STATE_IDLE;
        this.innerHTML = "";
      };
      if (sheet) {
        let done = false;
        const handler = () => {
          if (done) return;
          done = true;
          sheet.removeEventListener("transitionend", handler);
          finish();
        };
        sheet.addEventListener("transitionend", handler);
        // Belt-and-suspenders: 320ms covers --motion-duration-base
        // (200ms) plus margin even if the browser silently drops
        // the transitionend event.
        setTimeout(handler, 320);
      } else {
        finish();
      }

      if (!opts.skipPushState) {
        const url = new URL(window.location.href);
        if (url.searchParams.has("preview")) {
          url.searchParams.delete("preview");
          history.pushState({}, "", url);
        }
      }
    }

    _onKey(ev) {
      if (ev.key === "Escape" && this.dataset.state === STATE_OPEN) {
        ev.stopPropagation();
        this.close();
      }
    }

    _onPopState() {
      const params = new URLSearchParams(window.location.search);
      const preview = params.get("preview");
      if (preview) {
        if (preview !== this._currentID) {
          this.open(preview, { skipPushState: true });
        }
      } else if (this.dataset.state === STATE_OPEN) {
        this.close({ skipPushState: true });
      }
    }

    _onClick(ev) {
      // Underlay click: close.
      if (ev.target && ev.target.matches && ev.target.matches("[data-peek-underlay]")) {
        ev.preventDefault();
        this.close();
        return;
      }
      // Close button (or anything inside it) anywhere within the sheet.
      const closer = ev.target && ev.target.closest && ev.target.closest("[data-peek-close]");
      if (closer && this.contains(closer)) {
        ev.preventDefault();
        this.close();
      }
    }
  }

  customElements.define("peek-sheet", PeekSheet);

  // Convenience handle for other modules (the click-interception
  // module in 0KsRN will use this rather than re-querying the DOM).
  window.JobsPeek = {
    open(shortID) {
      const el = document.querySelector("peek-sheet");
      if (el) el.open(shortID);
    },
    close() {
      const el = document.querySelector("peek-sheet");
      if (el) el.close();
    },
  };
})();
