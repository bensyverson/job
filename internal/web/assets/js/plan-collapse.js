/*
  plan-collapse.js — interactive collapse/expand for the /plan tree.

  Click a .c-plan-row__disclosure button — or anywhere on the row body
  that isn't itself an interactive element — to flip data-collapsed on
  the row. CSS does the visual hiding; this module is just state.

  Whole-row toggle: a click on the row's title text, description, or
  empty chrome toggles the subtree. Clicks on links (the ID pill,
  blocker pills, label pills) and buttons fall through to their native
  behavior so peek and label-filter still work.

  Persistence:
  - Per-task collapse state lives in localStorage under jobs.plan.collapse
    as {shortID: true|false}. Absence means "follow the SSR default."
  - ?expand=<short>,<short> in the URL force-expands the listed rows
    on load and persists them, so a shareable link reveals exactly the
    branch the sender wants the recipient to see.

  Anchor support: when the URL hash is #task-<short> (a blocker pill
  click, a deep link, etc.), every ancestor row of that target is
  force-expanded so the row is actually visible. Runs on load and on
  every hashchange. Pairs with the in-page anchor markup added by
  kTfXu.
*/

(function () {
  "use strict";

  var STORAGE_KEY = "jobs.plan.collapse";

  // ---------- storage ----------

  function loadStorage() {
    try {
      var raw = localStorage.getItem(STORAGE_KEY);
      if (!raw) return {};
      var parsed = JSON.parse(raw);
      return parsed && typeof parsed === "object" ? parsed : {};
    } catch (_) {
      // Corrupt JSON or storage disabled — start clean rather than
      // throwing on every page load.
      return {};
    }
  }

  function saveStorage(state) {
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(state));
    } catch (_) {
      // localStorage may be unavailable (privacy mode, full quota).
      // The page still works; the next reload just loses the override.
    }
  }

  // ---------- DOM helpers ----------

  function rowFor(shortID) {
    if (!shortID) return null;
    // getElementById is safe — short ids are alphanumeric.
    return document.getElementById("task-" + shortID);
  }

  function setCollapsed(row, collapsed) {
    if (!row || !row.hasAttribute("data-collapsed")) return;
    var value = collapsed ? "true" : "false";
    if (row.getAttribute("data-collapsed") === value) return;
    row.setAttribute("data-collapsed", value);
    var btn = row.querySelector(":scope > .c-plan-row__disclosure");
    if (btn) {
      btn.setAttribute("aria-expanded", collapsed ? "false" : "true");
      btn.setAttribute("aria-label", collapsed ? "Expand" : "Collapse");
    }
  }

  // parentRowOf walks from a row up to its parent task row. Plan markup
  // nests as: <c-plan-row> [<c-plan-notes-group>] [<c-plan-subtree>] ...
  // A row inside a subtree's parent is the subtree's previous sibling
  // chain — skipping notes-group nodes that may sit between.
  function parentRowOf(row) {
    var subtree = row.parentElement;
    if (!subtree || !subtree.classList.contains("c-plan-subtree")) {
      return null;
    }
    var sib = subtree.previousElementSibling;
    while (sib && !sib.classList.contains("c-plan-row")) {
      sib = sib.previousElementSibling;
    }
    return sib || null;
  }

  function expandAncestorsOf(shortID, state) {
    var row = rowFor(shortID);
    if (!row) return;
    var ancestor = parentRowOf(row);
    while (ancestor) {
      var aShort = ancestor.getAttribute("data-plan-task");
      setCollapsed(ancestor, false);
      if (aShort) state[aShort] = false;
      ancestor = parentRowOf(ancestor);
    }
  }

  // ---------- hydration ----------

  function applyState(state) {
    Object.keys(state).forEach(function (shortID) {
      setCollapsed(rowFor(shortID), state[shortID] === true);
    });
  }

  function readURLOverrides() {
    var params = new URLSearchParams(window.location.search);
    var raw = params.get("expand");
    if (!raw) return [];
    return raw
      .split(",")
      .map(function (s) {
        return s.trim();
      })
      .filter(Boolean);
  }

  // ---------- click handler ----------

  function toggleRow(row) {
    if (!row) return;
    var shortID = row.getAttribute("data-plan-task");
    var collapsed = row.getAttribute("data-collapsed") === "true";
    setCollapsed(row, !collapsed);
    if (!shortID) return;
    var state = loadStorage();
    state[shortID] = !collapsed;
    saveStorage(state);
  }

  // A row click toggles when the user lands on inert chrome — title
  // text, description, the row's padding. We bail on interactive
  // descendants (links, buttons, inputs) so peek pills and label
  // pills keep working.
  function isInteractiveTarget(target) {
    return !!(target && target.closest && target.closest("a, button, input, summary, [data-peek]"));
  }

  function handleClick(event) {
    if (event.defaultPrevented) return;
    var disclosure = event.target.closest(".c-plan-row__disclosure");
    if (disclosure) {
      toggleRow(disclosure.closest(".c-plan-row"));
      return;
    }
    if (isInteractiveTarget(event.target)) return;
    var row = event.target.closest(".c-plan-row[data-collapsed]");
    if (!row) return;
    toggleRow(row);
  }

  // ---------- hash navigation ----------

  function shortIDFromHash() {
    var hash = window.location.hash || "";
    if (hash.indexOf("#task-") !== 0) return "";
    return hash.slice("#task-".length);
  }

  function revealHashTarget() {
    var shortID = shortIDFromHash();
    if (!shortID) return;
    var state = loadStorage();
    expandAncestorsOf(shortID, state);
    saveStorage(state);
    // The anchor itself is a no-op once the browser has already
    // jumped, but reassigning the hash re-triggers :target highlight
    // and scrolls to the row now that ancestors are visible.
    var row = rowFor(shortID);
    if (row && typeof row.scrollIntoView === "function") {
      row.scrollIntoView({ block: "center", behavior: "auto" });
    }
  }

  // ---------- init ----------

  function init() {
    if (!document.querySelector(".c-plan-row")) return; // not on /plan
    var state = loadStorage();
    var overrides = readURLOverrides();
    overrides.forEach(function (s) {
      state[s] = false;
    });
    if (overrides.length) saveStorage(state);
    applyState(state);
    revealHashTarget();
    document.addEventListener("click", handleClick);
    window.addEventListener("hashchange", revealHashTarget);
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init);
  } else {
    init();
  }

  // Exposed for plan-live.js to re-apply localStorage state to a
  // freshly-swapped fragment. Idempotent — safe to call repeatedly.
  window.JobsPlanCollapse = {
    applyStored: function () {
      applyState(loadStorage());
    },
  };
})();
