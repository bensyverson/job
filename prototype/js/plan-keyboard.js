/*
  Keyboard navigation for /plan. Roving tabindex pattern: at any
  moment, exactly one .c-plan-row is in the page's tab order
  (tabindex=0); the rest sit at tabindex=-1, focusable by script
  only. Tab into the section lands on the priming row; arrows then
  move the cursor row-by-row.

  Bindings (Dvorak-friendly — arrows primary, j/k as Vim aliases):
    ArrowDown / j     next visible row
    ArrowUp   / k     prev visible row
    ArrowLeft         collapse expanded row, else move to parent
    ArrowRight        expand collapsed row, else move to first child
    Space / Enter     toggle collapse on the focused row
    Home / End        first / last visible row

  Visibility check: collapsed subtrees use display: none, so
  offsetParent === null reliably distinguishes hidden rows from
  visible ones — no need to walk the ancestor chain.

  Self-guarded: a no-op if the page has no plan section. Bails when
  the focused element is an input/textarea/contenteditable so
  filter-bar typing doesn't get hijacked. Bails on modifier-key
  combos so browser shortcuts aren't trampled.

  Persistence: collapse toggles done via the keyboard write to the
  same localStorage key plan-collapse owns, so reload picks up the
  same state regardless of which surface drove the change.
*/

(function () {
  "use strict";

  var STORAGE_KEY = "jobs.plan.collapse";

  document.addEventListener("DOMContentLoaded", init);

  function init() {
    var section = document.querySelector("main .c-section[aria-label='Plan']");
    if (!section) return;
    primeFirstRow();
    document.addEventListener("keydown", onKeydown);
  }

  // ---------- helpers ----------

  function visibleRows() {
    return Array.from(
      document.querySelectorAll(".c-plan-row[data-plan-task]")
    ).filter(function (r) {
      return r.offsetParent !== null;
    });
  }

  function primeFirstRow() {
    var rows = visibleRows();
    if (rows.length === 0) return;
    var alreadyPrimed = rows.some(function (r) {
      return r.tabIndex === 0;
    });
    if (!alreadyPrimed) rows[0].tabIndex = 0;
  }

  function focusRow(row) {
    if (!row) return;
    document
      .querySelectorAll('.c-plan-row[tabindex="0"]')
      .forEach(function (r) {
        if (r !== row) r.tabIndex = -1;
      });
    row.tabIndex = 0;
    row.focus();
    if (typeof row.scrollIntoView === "function") {
      row.scrollIntoView({ block: "nearest" });
    }
  }

  function currentRow(rows) {
    var active = document.activeElement;
    if (
      active &&
      active.matches &&
      active.matches(".c-plan-row[data-plan-task]")
    ) {
      return active;
    }
    return rows[0] || null;
  }

  function isInputContext(el) {
    if (!el) return false;
    var tag = el.tagName;
    return tag === "INPUT" || tag === "TEXTAREA" || el.isContentEditable;
  }

  function isCollapsible(row) {
    return row.hasAttribute("data-collapsed");
  }

  function isCollapsed(row) {
    return row.getAttribute("data-collapsed") === "true";
  }

  function setCollapsed(row, collapsed) {
    if (!isCollapsible(row)) return;
    row.setAttribute("data-collapsed", collapsed ? "true" : "false");
    var btn = row.querySelector(":scope > .c-plan-row__disclosure");
    if (btn) {
      btn.setAttribute("aria-expanded", collapsed ? "false" : "true");
      btn.setAttribute("aria-label", collapsed ? "Expand" : "Collapse");
    }
    persistCollapse(row, collapsed);
  }

  function persistCollapse(row, collapsed) {
    try {
      var raw = localStorage.getItem(STORAGE_KEY);
      var state = raw ? JSON.parse(raw) : {};
      var id = row.getAttribute("data-plan-task");
      if (!id) return;
      state[id] = collapsed;
      localStorage.setItem(STORAGE_KEY, JSON.stringify(state));
    } catch (_) {
      // localStorage unavailable; visual change still applied.
    }
  }

  // parentRow walks from a row up to its parent task row. Plan markup
  // nests as: <c-plan-row> [<c-plan-notes-group>] [<c-plan-subtree>] …
  function parentRow(row) {
    var subtree = row.parentElement;
    if (!subtree || !subtree.classList.contains("c-plan-subtree")) return null;
    var sib = subtree.previousElementSibling;
    while (sib && !sib.classList.contains("c-plan-row")) {
      sib = sib.previousElementSibling;
    }
    return sib || null;
  }

  function firstChildRow(row) {
    var next = row.nextElementSibling;
    if (next && next.classList.contains("c-plan-notes-group")) {
      next = next.nextElementSibling;
    }
    if (!next || !next.classList.contains("c-plan-subtree")) return null;
    return (
      next.querySelector(":scope > .c-plan-row[data-plan-task]") || null
    );
  }

  // ---------- keydown ----------

  function onKeydown(ev) {
    if (isInputContext(ev.target)) return;
    if (ev.metaKey || ev.ctrlKey || ev.altKey) return;

    var rows = visibleRows();
    if (rows.length === 0) return;
    var cur = currentRow(rows);
    if (!cur) cur = rows[0];

    var handled = true;

    switch (ev.key) {
      case "ArrowDown":
      case "j": {
        var idx = rows.indexOf(cur);
        focusRow(rows[Math.min(rows.length - 1, idx + 1)]);
        break;
      }
      case "ArrowUp":
      case "k": {
        var idx2 = rows.indexOf(cur);
        focusRow(rows[Math.max(0, idx2 - 1)]);
        break;
      }
      case "ArrowLeft": {
        if (isCollapsible(cur) && !isCollapsed(cur)) {
          setCollapsed(cur, true);
        } else {
          var p = parentRow(cur);
          if (p) focusRow(p);
        }
        break;
      }
      case "ArrowRight": {
        if (isCollapsible(cur) && isCollapsed(cur)) {
          setCollapsed(cur, false);
        } else {
          var c = firstChildRow(cur);
          if (c) focusRow(c);
        }
        break;
      }
      case " ":
      case "Enter": {
        if (isCollapsible(cur)) setCollapsed(cur, !isCollapsed(cur));
        break;
      }
      case "Home": {
        focusRow(rows[0]);
        break;
      }
      case "End": {
        focusRow(rows[rows.length - 1]);
        break;
      }
      default:
        handled = false;
    }

    if (handled) ev.preventDefault();
  }
})();
