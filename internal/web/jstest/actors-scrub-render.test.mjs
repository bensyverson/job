// Tests for internal/web/assets/js/actors-scrub-render.mjs.
//
// Pure HTML emitter mirroring internal/web/templates/html/pages/
// actors.html.tmpl. Driver parses the result with DOMParser and swaps
// the live <div data-actors-board> in place.

import { test } from "node:test";
import assert from "node:assert/strict";

import { renderActorsBoard } from "../assets/js/actors-scrub-render.mjs";

function card(over = {}) {
  return {
    stateClass: "c-actor-card--claimed",
    verb: "claimed",
    verbClass: "c-log-row__verb--claimed",
    ageText: "5m",
    noteCount: 0,
    noteText: "",
    taskShortID: "T0001",
    taskURL: "/tasks/T0001",
    taskTitle: "Some task",
    taskDesc: "",
    isClaim: true,
    eventAt: 1700000000,
    cardKey: "alice:T0001",
    ...over,
  };
}

function col(over = {}) {
  return {
    name: "alice",
    url: "/actors/alice",
    idle: false,
    claimCount: 1,
    statusText: "1 claim · last seen 5m",
    cards: [card()],
    lastSeen: 1700000000,
    ...over,
  };
}

test("renderActorsBoard: empty cols emits the board host with no columns", () => {
  const html = renderActorsBoard([]);
  assert.match(html, /<div class="c-actors-board" aria-label="Actors" data-actors-board>/);
  assert.doesNotMatch(html, /c-actor-col/);
});

test("renderActorsBoard: column carries data-actor + name + status text", () => {
  const html = renderActorsBoard([col()]);
  assert.match(html, /data-actor="alice"/);
  assert.match(html, /id="a-alice"/);
  // Status text rendered verbatim.
  assert.match(html, />1 claim · last seen 5m</);
});

test("renderActorsBoard: idle column gets the c-actor-col--idle modifier", () => {
  const html = renderActorsBoard([col({ idle: true, claimCount: 0, statusText: "idle · last seen 1h", cards: [] })]);
  assert.match(html, /class="c-actor-col c-actor-col--idle"/);
  assert.match(html, /c-actor-col__status--idle/);
});

test("renderActorsBoard: card uses cardKey, eventAt, and stateClass; claim cards carry data-claim", () => {
  const html = renderActorsBoard([col()]);
  assert.match(html, /class="c-actor-card c-actor-card--claimed"/);
  assert.match(html, /data-actor-task="alice:T0001"/);
  assert.match(html, /data-event-at="1700000000"/);
  assert.match(html, /data-claim="1"/);
});

test("renderActorsBoard: history cards omit data-claim", () => {
  const html = renderActorsBoard([col({ cards: [card({ isClaim: false, stateClass: "c-actor-card--done", verb: "done", verbClass: "c-log-row__verb--done" })] })]);
  assert.doesNotMatch(html, /data-claim=/);
});

test("renderActorsBoard: noteText renders as the c-actor-card__notes badge with data-note-count", () => {
  const html = renderActorsBoard([col({ cards: [card({ noteCount: 3, noteText: "3 notes" })] })]);
  assert.match(html, /class="c-actor-card__notes" data-note-count="3"/);
  assert.match(html, />3 notes</);
});

test("renderActorsBoard: HTML-escapes title, description, and note text", () => {
  const html = renderActorsBoard([
    col({
      cards: [
        card({
          taskTitle: "T & U",
          taskDesc: "<script>alert('x')</script>",
        }),
      ],
    }),
  ]);
  assert.match(html, /T &amp; U/);
  assert.match(html, /&lt;script&gt;/);
  assert.doesNotMatch(html, /<script>alert/);
});

test("renderActorsBoard: data-peek link points at the task URL with aria-label", () => {
  const html = renderActorsBoard([col()]);
  assert.match(html, /href="\/tasks\/T0001" data-peek class="c-row-link" aria-label="Open task T0001"/);
});

test("renderActorsBoard: omits desc paragraph when taskDesc is empty", () => {
  const html = renderActorsBoard([col({ cards: [card({ taskDesc: "" })] })]);
  assert.doesNotMatch(html, /c-actor-card__desc/);
});

test("renderActorsBoard: emits desc paragraph when taskDesc is present", () => {
  const html = renderActorsBoard([col({ cards: [card({ taskDesc: "details" })] })]);
  assert.match(html, /<p class="c-actor-card__desc">details<\/p>/);
});
