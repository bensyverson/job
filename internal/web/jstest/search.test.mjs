// Tests for internal/web/assets/js/search.mjs.
//
// Pure-function layer of the header search dropdown. The DOM-bound
// controller (bindSearch) is exercised by hand in the browser; here we
// pin down the data shaping: HTML escaping, match highlighting, row
// rendering for both task and label kinds, keyboard nav, and the
// "/"-shortcut guard against typing in inputs.

import { test } from "node:test";
import assert from "node:assert/strict";

import {
  escapeHTML,
  highlightHTML,
  renderTaskRow,
  renderLabelRow,
  renderDropdown,
  nextIndex,
  shouldIgnoreSlash,
  pickEnterTarget,
} from "../assets/js/search.mjs";

// --- escapeHTML ---

test("escapeHTML escapes <, >, &, \", '", () => {
  assert.equal(escapeHTML(`<a href="x" class='y'>&</a>`),
    "&lt;a href=&quot;x&quot; class=&#39;y&#39;&gt;&amp;&lt;/a&gt;");
});

test("escapeHTML on empty/undefined returns empty string", () => {
  assert.equal(escapeHTML(""), "");
  assert.equal(escapeHTML(undefined), "");
  assert.equal(escapeHTML(null), "");
});

// --- highlightHTML ---

test("highlightHTML wraps case-insensitive matches in <strong>", () => {
  assert.equal(highlightHTML("Polish UI", "polish"),
    "<strong>Polish</strong> UI");
});

test("highlightHTML escapes surrounding HTML before wrapping matches", () => {
  assert.equal(highlightHTML("<b>foo</b> bar foo", "foo"),
    "&lt;b&gt;<strong>foo</strong>&lt;/b&gt; bar <strong>foo</strong>");
});

test("highlightHTML with empty query returns escaped text without <strong>", () => {
  assert.equal(highlightHTML("hello & world", ""), "hello &amp; world");
});

test("highlightHTML with no match returns escaped text", () => {
  assert.equal(highlightHTML("nothing here", "xyz"), "nothing here");
});

test("highlightHTML preserves case of original text inside <strong>", () => {
  assert.equal(highlightHTML("FooBar", "foo"), "<strong>Foo</strong>Bar");
});

// --- renderTaskRow ---

const taskRow = (over = {}) => ({
  kind: "task",
  short_id: "1SYqo",
  title: "Polish UI",
  status: "available",
  display_status: "todo",
  url: "/tasks/1SYqo",
  match_source: "title",
  excerpt: "",
  ...over,
});

test("renderTaskRow includes short_id, title, url, and status pill", () => {
  const html = renderTaskRow(taskRow(), "polish");
  assert.match(html, /data-url="\/tasks\/1SYqo"/);
  assert.match(html, /1SYqo/);
  assert.match(html, /<strong>Polish<\/strong> UI/);
  assert.match(html, /c-search-row__pill--todo/);
});

test("renderTaskRow omits excerpt for title and short_id matches", () => {
  for (const src of ["title", "short_id"]) {
    const html = renderTaskRow(taskRow({ match_source: src, excerpt: "should not appear" }), "polish");
    assert.doesNotMatch(html, /should not appear/);
    assert.doesNotMatch(html, /c-search-row__excerpt/);
  }
});

test("renderTaskRow includes excerpt for description matches", () => {
  const html = renderTaskRow(taskRow({
    match_source: "description",
    excerpt: "…the quick brown fox…",
    title: "T",
  }), "quick");
  assert.match(html, /c-search-row__excerpt/);
  assert.match(html, /<strong>quick<\/strong>/);
});

test("renderTaskRow escapes title and excerpt", () => {
  const html = renderTaskRow(taskRow({
    title: "<script>",
    match_source: "description",
    excerpt: "<b>x</b>",
  }), "");
  assert.match(html, /&lt;script&gt;/);
  assert.match(html, /&lt;b&gt;x&lt;\/b&gt;/);
  assert.doesNotMatch(html, /<script>/);
});

// --- renderLabelRow ---

test("renderLabelRow includes name, url, and label kind affordance", () => {
  const html = renderLabelRow({ kind: "label", name: "search", url: "/labels/search" }, "sea");
  assert.match(html, /data-url="\/labels\/search"/);
  assert.match(html, /<strong>sea<\/strong>rch/);
  assert.match(html, /c-search-row__kind/);
});

test("renderLabelRow escapes the label name", () => {
  const html = renderLabelRow({ kind: "label", name: "<x>", url: "/labels/%3Cx%3E" }, "");
  assert.match(html, /&lt;x&gt;/);
  assert.doesNotMatch(html, /<x>/);
});

// --- renderDropdown ---

test("renderDropdown returns empty string for empty query", () => {
  assert.equal(renderDropdown([], ""), "");
  assert.equal(renderDropdown([taskRow()], ""), "");
});

test("renderDropdown shows 'no matches' affordance when query is non-empty and results empty", () => {
  const html = renderDropdown([], "xyz");
  assert.match(html, /c-search-empty/);
  assert.match(html, /No matches/i);
});

test("renderDropdown dispatches between task and label rows by kind", () => {
  const html = renderDropdown([
    { kind: "label", name: "search", url: "/labels/search" },
    taskRow(),
  ], "sea");
  assert.match(html, /c-search-row__kind/);                          // label
  assert.match(html, /c-search-row__pill--todo/);                    // task
});

// --- nextIndex (keyboard navigation) ---

test("nextIndex clamps at the bottom (no wrap-around)", () => {
  assert.equal(nextIndex(2, 3, 1), 2);
  assert.equal(nextIndex(0, 3, 1), 1);
});

test("nextIndex clamps at the top", () => {
  assert.equal(nextIndex(0, 3, -1), 0);
  assert.equal(nextIndex(2, 3, -1), 1);
});

test("nextIndex with empty list returns -1", () => {
  assert.equal(nextIndex(-1, 0, 1), -1);
  assert.equal(nextIndex(-1, 0, -1), -1);
});

test("nextIndex from -1 (no selection) on ArrowDown selects first", () => {
  assert.equal(nextIndex(-1, 3, 1), 0);
});

// --- shouldIgnoreSlash ---

test("shouldIgnoreSlash returns true for INPUT, TEXTAREA, SELECT", () => {
  assert.equal(shouldIgnoreSlash({ tagName: "INPUT" }), true);
  assert.equal(shouldIgnoreSlash({ tagName: "TEXTAREA" }), true);
  assert.equal(shouldIgnoreSlash({ tagName: "SELECT" }), true);
});

test("shouldIgnoreSlash returns true for contentEditable elements", () => {
  assert.equal(shouldIgnoreSlash({ tagName: "DIV", isContentEditable: true }), true);
});

test("shouldIgnoreSlash returns false for a plain DIV / BUTTON / null", () => {
  assert.equal(shouldIgnoreSlash({ tagName: "DIV", isContentEditable: false }), false);
  assert.equal(shouldIgnoreSlash({ tagName: "BUTTON" }), false);
  assert.equal(shouldIgnoreSlash(null), false);
});

// --- pickEnterTarget ---

test("pickEnterTarget returns the active result when activeIndex >= 0", () => {
  const r = [{ url: "/a" }, { url: "/b" }];
  assert.equal(pickEnterTarget(r, 1), r[1]);
});

test("pickEnterTarget falls back to the first result when no active selection", () => {
  const r = [{ url: "/a" }, { url: "/b" }];
  assert.equal(pickEnterTarget(r, -1), r[0]);
});

test("pickEnterTarget returns null when results are empty", () => {
  assert.equal(pickEnterTarget([], -1), null);
  assert.equal(pickEnterTarget([], 0), null);
});
