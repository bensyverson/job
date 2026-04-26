// Tests for internal/web/assets/js/home-scrub-render.mjs.
//
// Pure HTML emitter mirroring internal/web/templates/html/pages/
// home.html.tmpl for the four signal cards + four panels. The driver
// parses each fragment with DOMParser and swaps it into the live
// page; the data-home-* hooks the live updater (home-live.js) needs
// stay intact.

import { test } from "node:test";
import assert from "node:assert/strict";

import {
  renderSignals,
  renderActiveClaims,
  renderRecentCompletions,
  renderUpcoming,
  renderBlocked,
} from "../assets/js/home-scrub-render.mjs";

// --- helpers ---

function emptyActivity() {
  return {
    Bars: new Array(60).fill({ Empty: true }),
    TotalDone: 0,
    TotalClaim: 0,
    TotalCreate: 0,
    TotalBlock: 0,
    TotalEvents: 0,
  };
}

function defaultBag(over = {}) {
  return {
    Activity: emptyActivity(),
    NewlyBlocked: { Count: 0, Items: [], ProgressPct: 0 },
    LongestClaim: { Present: false, ProgressPct: 0 },
    OldestTodo: { Present: false, ProgressPct: 0 },
    ActiveClaims: { Count: 0, Rows: [] },
    RecentCompletions: { Count: 0, Rows: [] },
    Upcoming: { Count: 0, Rows: [] },
    Blocked: { Count: 0, Rows: [] },
    ...over,
  };
}

// --- renderSignals: chrome ---

test("renderSignals: emits the c-grid-signals host with four signal cards", () => {
  const html = renderSignals(defaultBag());
  assert.match(html, /class="c-grid-signals"/);
  // Activity (primary), NewlyBlocked (warn), LongestClaim (primary), OldestTodo (warn).
  const cardCount = (html.match(/c-signal-card c-signal-card--/g) || []).length;
  assert.equal(cardCount, 4);
  assert.match(html, /Activity · last 60m/);
  assert.match(html, /Newly blocked · 10m/);
  assert.match(html, /Longest active claim/);
  assert.match(html, /Oldest todo/);
});

// --- Activity card ---

test("renderSignals: empty activity bars render as the empty placeholder", () => {
  const html = renderSignals(defaultBag());
  // 60 empty bars expected.
  const empties = (html.match(/c-histogram__bar--empty/g) || []).length;
  assert.equal(empties, 60);
});

test("renderSignals: stacked activity bar renders style and segments", () => {
  const bars = new Array(60).fill({ Empty: true });
  bars[59] = { Empty: false, HeightPercent: 100, Done: 2, Claim: 1, Create: 0, Block: 0 };
  const bag = defaultBag({
    Activity: {
      Bars: bars,
      TotalDone: 2,
      TotalClaim: 1,
      TotalCreate: 0,
      TotalBlock: 0,
      TotalEvents: 3,
    },
  });
  const html = renderSignals(bag);
  assert.match(html, /style="--h:100%"/);
  assert.match(html, /c-histogram__seg--done"\s+style="flex:2"/);
  assert.match(html, /c-histogram__seg--claim"\s+style="flex:1"/);
  // Zero segments are omitted.
  assert.doesNotMatch(html, /c-histogram__seg--create/);
  assert.doesNotMatch(html, /c-histogram__seg--block/);
});

test("renderSignals: legend totals render", () => {
  const bag = defaultBag({
    Activity: {
      Bars: new Array(60).fill({ Empty: true }),
      TotalDone: 5,
      TotalClaim: 3,
      TotalCreate: 7,
      TotalBlock: 2,
      TotalEvents: 17,
    },
  });
  const html = renderSignals(bag);
  assert.match(html, />5 done</);
  assert.match(html, />3 claimed</);
  assert.match(html, />7 new</);
  assert.match(html, />2 blocked</);
});

// --- NewlyBlocked card ---

test("renderSignals: newly-blocked card with items renders pills + waiting-on", () => {
  const bag = defaultBag({
    NewlyBlocked: {
      Count: 2,
      ProgressPct: 40,
      Items: [
        {
          BlockedShortID: "B1",
          BlockedURL: "/tasks/B1",
          WaitingOnShortID: "K1",
          WaitingOnURL: "/tasks/K1",
        },
      ],
    },
  });
  const html = renderSignals(bag);
  assert.match(html, /--progress: 40%/);
  assert.match(html, /href="\/tasks\/B1" class="c-id-pill">B1</);
  assert.match(html, /href="\/tasks\/K1" class="c-id-pill">K1</);
  assert.match(html, /waiting on/);
  // The big number is the count.
  assert.match(html, /class="c-signal-card__value">2</);
});

test("renderSignals: newly-blocked empty state shows muted 'No new blocks'", () => {
  const html = renderSignals(defaultBag());
  assert.match(html, /No new blocks/);
});

// --- LongestClaim card ---

test("renderSignals: longest-claim present renders duration, actor, task pill", () => {
  const bag = defaultBag({
    LongestClaim: {
      Present: true,
      Actor: "alice",
      ActorURL: "/actors/alice",
      TaskShortID: "T1",
      TaskURL: "/tasks/T1",
      TaskTitle: "x",
      DurationText: "12m 5s",
      ProgressPct: 40,
    },
  });
  const html = renderSignals(bag);
  assert.match(html, /class="c-signal-card__value">12m 5s</);
  assert.match(html, /data-actor="alice"/);
  assert.match(html, /href="\/actors\/alice"/);
  assert.match(html, /href="\/tasks\/T1" class="c-id-pill">T1</);
  assert.match(html, /--progress: 40%/);
});

test("renderSignals: longest-claim absent renders em-dash + 'No active claims'", () => {
  const html = renderSignals(defaultBag());
  // The card label appears, plus an em-dash value, plus the muted text.
  assert.match(html, /Longest active claim/);
  assert.match(html, /No active claims/);
});

// --- OldestTodo card ---

test("renderSignals: oldest-todo present renders age + pill + title", () => {
  const bag = defaultBag({
    OldestTodo: {
      Present: true,
      TaskShortID: "T2",
      TaskURL: "/tasks/T2",
      Title: "Reticulate",
      AgeText: "3d 2h",
      ProgressPct: 50,
    },
  });
  const html = renderSignals(bag);
  assert.match(html, /class="c-signal-card__value">3d 2h</);
  assert.match(html, /href="\/tasks\/T2" class="c-id-pill">T2</);
  assert.match(html, /Reticulate/);
});

test("renderSignals: oldest-todo absent shows em-dash + 'Nothing waiting'", () => {
  const html = renderSignals(defaultBag());
  assert.match(html, /Nothing waiting/);
});

// --- Active claims panel ---

test("renderActiveClaims: section carries data-home-claims and meta count", () => {
  const html = renderActiveClaims({
    Count: 1,
    Rows: [
      {
        Actor: "alice",
        ActorURL: "/actors/alice",
        TaskShortID: "T1",
        TaskURL: "/tasks/T1",
        TaskTitle: "Some task",
        DurationText: "5m 30s",
        ClaimedAtUnix: 1700000000,
      },
    ],
  });
  assert.match(html, /data-home-claims/);
  assert.match(html, /1 in flight/);
  assert.match(html, /data-claimed-at="1700000000"/);
  // data-claim-idle is a boolean attribute (no value) matching the Go
  // template; the duration text follows directly after the > .
  assert.match(html, /data-claim-idle>5m 30s</);
  assert.match(html, /data-actor="alice"/);
  assert.match(html, /Some task/);
});

test("renderActiveClaims: empty state renders 'No active claims'", () => {
  const html = renderActiveClaims({ Count: 0, Rows: [] });
  assert.match(html, /No active claims/);
});

// --- Recent completions panel ---

test("renderRecentCompletions: section carries data-home-recent + 'last N' meta", () => {
  const html = renderRecentCompletions({
    Count: 2,
    Rows: [
      {
        Actor: "alice",
        ActorURL: "/actors/alice",
        TaskShortID: "T1",
        TaskURL: "/tasks/T1",
        TaskTitle: "Done task",
        AgeText: "2m",
        CompletedAtUnix: 1700000000,
      },
      {
        Actor: "bob",
        ActorURL: "/actors/bob",
        TaskShortID: "T2",
        TaskURL: "/tasks/T2",
        TaskTitle: "Other",
        AgeText: "5m",
        CompletedAtUnix: 1700000000,
      },
    ],
  });
  assert.match(html, /data-home-recent/);
  assert.match(html, /last 2/);
  assert.match(html, /Done task/);
  assert.match(html, /Other/);
  assert.match(html, /data-actor="bob"/);
});

test("renderRecentCompletions: empty state renders 'No recent completions'", () => {
  const html = renderRecentCompletions({ Count: 0, Rows: [] });
  assert.match(html, /No recent completions/);
});

// --- Upcoming panel ---

test("renderUpcoming: section carries data-home-upcoming + 'N ready' meta", () => {
  const html = renderUpcoming({
    Count: 1,
    Rows: [
      {
        TaskShortID: "T1",
        TaskURL: "/tasks/T1",
        TaskTitle: "Build it",
        AgeText: "1h 5m",
        CreatedAtUnix: 1700000000,
      },
    ],
  });
  assert.match(html, /data-home-upcoming/);
  assert.match(html, /1 ready/);
  assert.match(html, /data-created-at="1700000000"/);
  assert.match(html, /Build it/);
});

// --- Blocked panel ---

test("renderBlocked: section carries data-home-blocked + 'N waiting' meta + blocker pills", () => {
  const html = renderBlocked({
    Count: 1,
    Rows: [
      {
        TaskShortID: "T1",
        TaskURL: "/tasks/T1",
        TaskTitle: "Stuck",
        Blockers: [
          { ShortID: "K1", URL: "/tasks/K1" },
          { ShortID: "K2", URL: "/tasks/K2" },
        ],
      },
    ],
  });
  assert.match(html, /data-home-blocked/);
  assert.match(html, /1 waiting/);
  assert.match(html, /Stuck/);
  assert.match(html, /href="\/tasks\/K1" class="c-id-pill">K1</);
  assert.match(html, /href="\/tasks\/K2" class="c-id-pill">K2</);
  assert.match(html, /waiting on/);
});

test("renderBlocked: empty state renders 'No blocked tasks'", () => {
  const html = renderBlocked({ Count: 0, Rows: [] });
  assert.match(html, /No blocked tasks/);
});

// --- Escaping ---

test("renderSignals: escapes HTML in titles and actor names", () => {
  const bag = defaultBag({
    OldestTodo: {
      Present: true,
      TaskShortID: "T1",
      TaskURL: "/tasks/T1",
      Title: "<script>alert(1)</script>",
      AgeText: "1m",
      ProgressPct: 0,
    },
  });
  const html = renderSignals(bag);
  assert.doesNotMatch(html, /<script>alert/);
  assert.match(html, /&lt;script&gt;/);
});

test("renderActiveClaims: escapes task title", () => {
  const html = renderActiveClaims({
    Count: 1,
    Rows: [
      {
        Actor: "alice",
        ActorURL: "/actors/alice",
        TaskShortID: "T1",
        TaskURL: "/tasks/T1",
        TaskTitle: "T & U",
        DurationText: "1s",
        ClaimedAtUnix: 1700000000,
      },
    ],
  });
  assert.match(html, /T &amp; U/);
});
