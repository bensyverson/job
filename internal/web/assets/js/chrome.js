/*
  Chrome behavior the prototype HTML doesn't cover: theme toggle and
  the "/" keyboard shortcut that focuses the header search input.
  colors.js handles actor/label painting; this file handles the rest.
*/

(function () {
  "use strict";

  var root = document.documentElement;
  var THEME_KEY = "jobs.theme";

  function applyTheme(theme) {
    if (theme === "light") {
      root.setAttribute("data-theme", "light");
    } else {
      root.removeAttribute("data-theme");
    }
  }

  function initTheme() {
    var stored = localStorage.getItem(THEME_KEY);
    if (stored === "dark" || stored === "light") {
      applyTheme(stored);
      return;
    }
    var prefersLight =
      window.matchMedia &&
      window.matchMedia("(prefers-color-scheme: light)").matches;
    applyTheme(prefersLight ? "light" : "dark");
  }

  function initThemeToggle() {
    var btn = document.querySelector("[data-theme-toggle]");
    if (!btn) return;
    btn.addEventListener("click", function () {
      var next = root.getAttribute("data-theme") === "light" ? "dark" : "light";
      applyTheme(next);
      localStorage.setItem(THEME_KEY, next);
    });
  }

  function initSearchFocus() {
    var input = document.querySelector("[data-search-input]");
    if (!input) return;
    document.addEventListener("keydown", function (ev) {
      if (ev.key !== "/") return;
      var t = ev.target;
      if (t instanceof HTMLElement) {
        var tag = t.tagName;
        if (tag === "INPUT" || tag === "TEXTAREA" || t.isContentEditable) return;
      }
      ev.preventDefault();
      input.focus();
      input.select();
    });
  }

  initTheme();
  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", function () {
      initThemeToggle();
      initSearchFocus();
    });
  } else {
    initThemeToggle();
    initSearchFocus();
  }
})();
