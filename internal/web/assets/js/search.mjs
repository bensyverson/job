// Header search dropdown.
//
// Pure helpers (escapeHTML, highlightHTML, render*, nextIndex,
// shouldIgnoreSlash, pickEnterTarget) are exported and unit-tested in
// internal/web/jstest/search.test.mjs. bindSearch wires the live DOM:
// "/" focuses the input from anywhere on the page (guarded against
// inputs/textareas/contentEditable), debounced fetch against /search,
// dropdown render, ↑/↓/Enter/Escape navigation. Module bootstraps
// itself on DOMContentLoaded when running in a browser.

const HTML_ESCAPES = {
  "&": "&amp;",
  "<": "&lt;",
  ">": "&gt;",
  '"': "&quot;",
  "'": "&#39;",
};

export function escapeHTML(s) {
  if (s == null) return "";
  return String(s).replace(/[&<>"']/g, (c) => HTML_ESCAPES[c]);
}

export function highlightHTML(text, query) {
  if (!text) return "";
  const t = String(text);
  const q = String(query || "");
  if (!q) return escapeHTML(t);
  const lower = t.toLowerCase();
  const ql = q.toLowerCase();
  let out = "";
  let i = 0;
  while (i < t.length) {
    const idx = lower.indexOf(ql, i);
    if (idx < 0) {
      out += escapeHTML(t.slice(i));
      break;
    }
    out += escapeHTML(t.slice(i, idx));
    out += "<strong>" + escapeHTML(t.slice(idx, idx + ql.length)) + "</strong>";
    i = idx + ql.length;
  }
  return out;
}

function showExcerpt(matchSource) {
  return matchSource && matchSource !== "title" && matchSource !== "short_id";
}

export function renderTaskRow(r, query) {
  const pillClass = "c-search-row__pill--" + escapeHTML(r.display_status || "todo");
  const excerptHTML = showExcerpt(r.match_source) && r.excerpt
    ? `<span class="c-search-row__excerpt">${highlightHTML(r.excerpt, query)}</span>`
    : "";
  return `<li class="c-search-row" role="option" data-search-row data-url="${escapeHTML(r.url)}">`
    + `<div class="c-search-row__main">`
    +   `<span class="c-search-row__pill ${pillClass}">${escapeHTML(r.display_status)}</span>`
    +   `<span class="c-search-row__id">${highlightHTML(r.short_id, query)}</span>`
    +   `<span class="c-search-row__title">${highlightHTML(r.title, query)}</span>`
    + `</div>`
    + excerptHTML
    + `</li>`;
}

export function renderLabelRow(r, query) {
  return `<li class="c-search-row c-search-row--label" role="option" data-search-row data-url="${escapeHTML(r.url)}">`
    + `<div class="c-search-row__main">`
    +   `<span class="c-search-row__kind">label</span>`
    +   `<span class="c-search-row__title">${highlightHTML(r.name, query)}</span>`
    + `</div>`
    + `</li>`;
}

export function renderDropdown(results, query) {
  if (!query) return "";
  if (!Array.isArray(results) || results.length === 0) {
    return `<li class="c-search-empty" role="option" aria-disabled="true">No matches</li>`;
  }
  return results
    .map((r) => (r.kind === "label" ? renderLabelRow(r, query) : renderTaskRow(r, query)))
    .join("");
}

export function nextIndex(curr, total, dir) {
  if (total <= 0) return -1;
  if (dir > 0) {
    if (curr < 0) return 0;
    return Math.min(curr + 1, total - 1);
  }
  if (curr < 0) return 0;
  return Math.max(curr - 1, 0);
}

export function shouldIgnoreSlash(target) {
  if (!target) return false;
  const tag = target.tagName;
  if (tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT") return true;
  if (target.isContentEditable) return true;
  return false;
}

export function pickEnterTarget(results, activeIndex) {
  if (!Array.isArray(results) || results.length === 0) return null;
  if (activeIndex >= 0 && activeIndex < results.length) return results[activeIndex];
  return results[0];
}

export function bindSearch(opts) {
  const doc = opts.document;
  const fetcher = opts.fetch;
  const navigate = opts.navigate;
  const debounceMs = opts.debounceMs == null ? 150 : opts.debounceMs;

  const root = doc.querySelector("[data-search-root]");
  if (!root) return;
  const input = root.querySelector("[data-search-input]");
  const list = root.querySelector("[data-search-results]");
  if (!input || !list) return;

  let timer = null;
  let seq = 0;
  let activeIndex = -1;
  let results = [];
  let currentQuery = "";

  function close() {
    list.hidden = true;
    list.innerHTML = "";
    activeIndex = -1;
    results = [];
  }

  function setActive(i) {
    activeIndex = i;
    const rows = list.querySelectorAll("[data-search-row]");
    rows.forEach((row, idx) => row.classList.toggle("c-search-row--active", idx === i));
    if (i >= 0 && rows[i] && typeof rows[i].scrollIntoView === "function") {
      rows[i].scrollIntoView({ block: "nearest" });
    }
  }

  function render() {
    list.innerHTML = renderDropdown(results, currentQuery);
    list.hidden = false;
    setActive(results.length > 0 ? 0 : -1);
  }

  async function runQuery(q) {
    const mySeq = ++seq;
    if (!q) { close(); return; }
    try {
      const resp = await fetcher("/search?q=" + encodeURIComponent(q) + "&limit=20");
      if (mySeq !== seq) return;
      const data = await resp.json();
      results = Array.isArray(data) ? data : [];
      currentQuery = q;
      render();
    } catch {
      // network errors are silent — the dropdown just won't update
    }
  }

  input.addEventListener("input", () => {
    const q = input.value.trim();
    currentQuery = q;
    if (timer != null) clearTimeout(timer);
    timer = setTimeout(() => runQuery(q), debounceMs);
  });

  input.addEventListener("keydown", (e) => {
    if (e.key === "Escape") {
      close();
      input.blur();
      e.preventDefault();
      return;
    }
    if (e.key === "ArrowDown") {
      e.preventDefault();
      setActive(nextIndex(activeIndex, results.length, 1));
      return;
    }
    if (e.key === "ArrowUp") {
      e.preventDefault();
      setActive(nextIndex(activeIndex, results.length, -1));
      return;
    }
    if (e.key === "Enter") {
      const target = pickEnterTarget(results, activeIndex);
      if (target && target.url) {
        e.preventDefault();
        navigate(target.url);
      }
    }
  });

  doc.addEventListener("keydown", (e) => {
    if (e.key !== "/" || e.metaKey || e.ctrlKey || e.altKey) return;
    if (shouldIgnoreSlash(e.target)) return;
    e.preventDefault();
    input.focus();
    input.select();
  });

  doc.addEventListener("click", (e) => {
    const row = e.target && e.target.closest && e.target.closest("[data-search-row]");
    if (row) {
      const url = row.getAttribute("data-url");
      if (url) navigate(url);
      return;
    }
    if (!root.contains(e.target)) close();
  });
}

if (typeof document !== "undefined") {
  document.addEventListener("DOMContentLoaded", () => {
    bindSearch({
      document,
      fetch: window.fetch.bind(window),
      navigate: (url) => { window.location.href = url; },
    });
  });
}
