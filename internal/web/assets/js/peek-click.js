/*
  Task-click interception. A single document-level delegator that
  catches clicks on `<a data-peek href="/tasks/<id>…">` and routes
  them through the peek sheet instead of triggering a hard navigation.

  Progressive-enhancement contract:

    - Without JS, every <a> is a normal link to /tasks/<id>.
    - With JS, the delegator opens the peek sheet for that id.
    - Cmd / Ctrl / Shift / Alt clicks pass through to native nav so
      "open in new tab", "save link", and similar OS gestures work.
    - Aux clicks (middle button) and right-clicks pass through.
    - Links pointing outside /tasks/<id> are ignored.

  Resolves the task id by parsing the href, not by reading a
  data-peek="<id>" attribute, so server templates only need a single
  attribute switch (data-peek) instead of duplicating the id.
*/

(function () {
  "use strict";

  document.addEventListener("DOMContentLoaded", init);

  function init() {
    document.addEventListener("click", onClick);
  }

  function onClick(ev) {
    if (ev.defaultPrevented) return;
    if (ev.button !== 0) return; // only primary button — middle/right pass through
    if (ev.metaKey || ev.ctrlKey || ev.shiftKey || ev.altKey) return;

    const link = ev.target && ev.target.closest && ev.target.closest("a[data-peek]");
    if (!link) return;

    const id = parseTaskID(link.getAttribute("href") || "");
    if (!id) return;

    if (!window.JobsPeek || typeof window.JobsPeek.open !== "function") {
      // No peek module loaded — let the native navigation happen.
      return;
    }

    ev.preventDefault();
    window.JobsPeek.open(id);
  }

  // parseTaskID extracts the short id from "/tasks/<id>" or
  // "/tasks/<id>/anything". Returns "" when the href doesn't match.
  // Stripping the origin lets the regex run against absolute URLs
  // (e.g. https://host/tasks/abc) as well as path-only hrefs.
  function parseTaskID(href) {
    let path = href;
    if (path.indexOf("://") >= 0) {
      try {
        path = new URL(path).pathname;
      } catch (_) {
        return "";
      }
    }
    // Strip trailing query / hash before matching.
    const q = path.indexOf("?");
    if (q >= 0) path = path.slice(0, q);
    const h = path.indexOf("#");
    if (h >= 0) path = path.slice(0, h);

    const m = path.match(/^\/tasks\/([^\/]+)\/?$/);
    return m ? m[1] : "";
  }
})();
