// Tests for internal/web/assets/js/replay.mjs.
//
// Runs under Node's built-in test runner (node --test).
// No browser, no DOM, no deps — the module under test is pure data.
//
// The reducer fold is event-table-driven; each test fixture mirrors a
// single event the server actually produces (verified against samples
// from the local .jobs.db on 2026-04-26). Round-trip tests verify
// applyEvent / reverseEvent compose to identity for every event type
// where the server records the breadcrumbs (post-commit 915916d).

import { test } from "node:test";
import assert from "node:assert/strict";

import {
  initialFrame,
  applyEvent,
  reverseEvent,
  FrameCache,
} from "../assets/js/replay.mjs";

// --- helpers ---

// Minimal initial frame factory for tests. Lets each test name only
// the slots it cares about; everything else defaults to empty.
function emptyFrame(eventId = 0) {
  return initialFrame({
    headEventId: eventId,
    tasks: [],
    blocks: [],
    claims: [],
  });
}

// Frames are deep-equal'd field-by-field. Maps and Sets need
// normalization since assert.deepStrictEqual treats them by reference
// in some edge cases — converting to plain objects is robust.
function normalize(frame) {
  return {
    eventId: frame.eventId,
    tasks: Object.fromEntries(
      [...frame.tasks].map(([k, v]) => [
        k,
        { ...v, labels: [...(v.labels ?? [])].sort() },
      ]),
    ),
    blocks: Object.fromEntries(
      [...frame.blocks].map(([k, set]) => [k, [...set].sort()]),
    ),
    claims: Object.fromEntries(frame.claims),
  };
}

function assertFramesEqual(actual, expected, msg) {
  assert.deepStrictEqual(normalize(actual), normalize(expected), msg);
}

// --- initialFrame ---

test("initialFrame: empty payload produces an empty frame", () => {
  const f = initialFrame({
    headEventId: 0,
    tasks: [],
    blocks: [],
    claims: [],
  });
  assert.equal(f.eventId, 0);
  assert.equal(f.tasks.size, 0);
  assert.equal(f.blocks.size, 0);
  assert.equal(f.claims.size, 0);
});

test("initialFrame: hydrates tasks, blocks, claims from the JSON island", () => {
  const f = initialFrame({
    headEventId: 42,
    tasks: [
      {
        shortId: "ABC12",
        title: "Task A",
        description: "desc",
        status: "available",
        parentShortId: null,
        sortOrder: 1,
        labels: ["web", "dx"],
      },
    ],
    blocks: [{ blockedShortId: "ABC12", blockerShortId: "XYZ99" }],
    claims: [{ shortId: "ABC12", claimedBy: "alice", expiresAt: 1000 }],
  });

  assert.equal(f.eventId, 42);
  assert.equal(f.tasks.size, 1);
  const t = f.tasks.get("ABC12");
  assert.equal(t.title, "Task A");
  assert.deepStrictEqual([...t.labels].sort(), ["dx", "web"]);
  assert.deepStrictEqual([...f.blocks.get("ABC12")], ["XYZ99"]);
  assert.deepStrictEqual(f.claims.get("ABC12"), {
    claimedBy: "alice",
    expiresAt: 1000,
  });
});

// --- forward fold per event type ---

test("applyEvent created: inserts task with title/desc/parent/sortOrder", () => {
  const before = emptyFrame(0);
  const event = {
    id: 1,
    task_id: "ABC12",
    actor: "alice",
    event_type: "created",
    detail: {
      title: "New task",
      description: "desc",
      parent_id: "",
      sort_order: 5,
    },
  };

  const after = applyEvent(before, event);
  const t = after.tasks.get("ABC12");
  assert.equal(t.title, "New task");
  assert.equal(t.description, "desc");
  assert.equal(t.parentShortId, null);
  assert.equal(t.sortOrder, 5);
  assert.equal(t.status, "available");
  assert.equal(after.eventId, 1);
});

test("applyEvent claimed: sets claim with expires_at; status -> claimed", () => {
  const before = initialFrame({
    headEventId: 0,
    tasks: [{ shortId: "ABC12", title: "T", status: "available", sortOrder: 0 }],
    blocks: [],
    claims: [],
  });
  const after = applyEvent(before, {
    id: 2,
    task_id: "ABC12",
    actor: "alice",
    event_type: "claimed",
    detail: { duration: "30m", expires_at: 1500 },
  });

  assert.equal(after.tasks.get("ABC12").status, "claimed");
  assert.deepStrictEqual(after.claims.get("ABC12"), {
    claimedBy: "alice",
    expiresAt: 1500,
  });
});

test("applyEvent released: clears claim; status -> available", () => {
  const before = initialFrame({
    headEventId: 0,
    tasks: [{ shortId: "ABC12", title: "T", status: "claimed", sortOrder: 0 }],
    blocks: [],
    claims: [{ shortId: "ABC12", claimedBy: "alice", expiresAt: 1500 }],
  });
  const after = applyEvent(before, {
    id: 3,
    task_id: "ABC12",
    actor: "alice",
    event_type: "released",
    detail: { was_claimed_by: "alice", was_expires_at: 1500 },
  });

  assert.equal(after.tasks.get("ABC12").status, "available");
  assert.equal(after.claims.has("ABC12"), false);
});

test("applyEvent done: status -> done; clears any claim", () => {
  const before = initialFrame({
    headEventId: 0,
    tasks: [{ shortId: "ABC12", title: "T", status: "claimed", sortOrder: 0 }],
    blocks: [],
    claims: [{ shortId: "ABC12", claimedBy: "alice", expiresAt: 1500 }],
  });
  const after = applyEvent(before, {
    id: 4,
    task_id: "ABC12",
    actor: "alice",
    event_type: "done",
    detail: {
      cascade: false,
      was_status: "claimed",
      was_claimed_by: "alice",
      was_expires_at: 1500,
    },
  });

  assert.equal(after.tasks.get("ABC12").status, "done");
  assert.equal(after.claims.has("ABC12"), false);
});

test("applyEvent canceled: status -> canceled; clears any claim", () => {
  const before = initialFrame({
    headEventId: 0,
    tasks: [{ shortId: "ABC12", title: "T", status: "claimed", sortOrder: 0 }],
    blocks: [],
    claims: [{ shortId: "ABC12", claimedBy: "alice", expiresAt: 1500 }],
  });
  const after = applyEvent(before, {
    id: 5,
    task_id: "ABC12",
    actor: "alice",
    event_type: "canceled",
    detail: {
      cascade: false,
      was_status: "claimed",
      was_claimed_by: "alice",
      was_expires_at: 1500,
    },
  });

  assert.equal(after.tasks.get("ABC12").status, "canceled");
  assert.equal(after.claims.has("ABC12"), false);
});

test("applyEvent reopened: status -> available", () => {
  const before = initialFrame({
    headEventId: 0,
    tasks: [{ shortId: "ABC12", title: "T", status: "done", sortOrder: 0 }],
    blocks: [],
    claims: [],
  });
  const after = applyEvent(before, {
    id: 6,
    task_id: "ABC12",
    actor: "alice",
    event_type: "reopened",
    detail: { from_status: "done" },
  });

  assert.equal(after.tasks.get("ABC12").status, "available");
});

test("applyEvent blocked: adds edge to blocks map", () => {
  const before = initialFrame({
    headEventId: 0,
    tasks: [
      { shortId: "ABC12", title: "T", status: "available", sortOrder: 0 },
      { shortId: "XYZ99", title: "B", status: "available", sortOrder: 0 },
    ],
    blocks: [],
    claims: [],
  });
  const after = applyEvent(before, {
    id: 7,
    task_id: "ABC12",
    actor: "alice",
    event_type: "blocked",
    detail: { blocked_id: "ABC12", blocker_id: "XYZ99" },
  });

  assert.deepStrictEqual([...after.blocks.get("ABC12")], ["XYZ99"]);
});

test("applyEvent unblocked: removes edge from blocks map", () => {
  const before = initialFrame({
    headEventId: 0,
    tasks: [
      { shortId: "ABC12", title: "T", status: "available", sortOrder: 0 },
      { shortId: "XYZ99", title: "B", status: "available", sortOrder: 0 },
    ],
    blocks: [{ blockedShortId: "ABC12", blockerShortId: "XYZ99" }],
    claims: [],
  });
  const after = applyEvent(before, {
    id: 8,
    task_id: "ABC12",
    actor: "alice",
    event_type: "unblocked",
    detail: { blocked_id: "ABC12", blocker_id: "XYZ99", reason: "blocker_done" },
  });

  assert.equal(after.blocks.has("ABC12"), false);
});

test("applyEvent labeled: adds names; existing labels untouched", () => {
  const before = initialFrame({
    headEventId: 0,
    tasks: [
      {
        shortId: "ABC12",
        title: "T",
        status: "available",
        sortOrder: 0,
        labels: ["existing"],
      },
    ],
    blocks: [],
    claims: [],
  });
  const after = applyEvent(before, {
    id: 9,
    task_id: "ABC12",
    actor: "alice",
    event_type: "labeled",
    detail: { names: ["web", "dx"], existing: ["existing"] },
  });

  assert.deepStrictEqual(
    [...after.tasks.get("ABC12").labels].sort(),
    ["dx", "existing", "web"],
  );
});

test("applyEvent edited: updates title and/or description", () => {
  const before = initialFrame({
    headEventId: 0,
    tasks: [
      {
        shortId: "ABC12",
        title: "old title",
        description: "old desc",
        status: "available",
        sortOrder: 0,
      },
    ],
    blocks: [],
    claims: [],
  });
  const after = applyEvent(before, {
    id: 10,
    task_id: "ABC12",
    actor: "alice",
    event_type: "edited",
    detail: {
      old_title: "old title",
      new_title: "new title",
      old_desc: "old desc",
      new_desc: "new desc",
    },
  });

  const t = after.tasks.get("ABC12");
  assert.equal(t.title, "new title");
  assert.equal(t.description, "new desc");
});

test("applyEvent moved: updates sortOrder", () => {
  const before = initialFrame({
    headEventId: 0,
    tasks: [
      { shortId: "ABC12", title: "T", status: "available", sortOrder: 5 },
    ],
    blocks: [],
    claims: [],
  });
  const after = applyEvent(before, {
    id: 11,
    task_id: "ABC12",
    actor: "alice",
    event_type: "moved",
    detail: {
      direction: "after",
      relative_to: "XYZ99",
      old_sort_order: 5,
      new_sort_order: 10,
    },
  });

  assert.equal(after.tasks.get("ABC12").sortOrder, 10);
});

test("applyEvent noted: replaces description with description_after", () => {
  const before = initialFrame({
    headEventId: 0,
    tasks: [
      {
        shortId: "ABC12",
        title: "T",
        description: "before",
        status: "available",
        sortOrder: 0,
      },
    ],
    blocks: [],
    claims: [],
  });
  const after = applyEvent(before, {
    id: 12,
    task_id: "ABC12",
    actor: "alice",
    event_type: "noted",
    detail: {
      description_after: "before\n\n[note] after",
      text: "after",
    },
  });

  assert.equal(after.tasks.get("ABC12").description, "before\n\n[note] after");
});

test("applyEvent claim_expired: clears claim; status -> available", () => {
  const before = initialFrame({
    headEventId: 0,
    tasks: [{ shortId: "ABC12", title: "T", status: "claimed", sortOrder: 0 }],
    blocks: [],
    claims: [{ shortId: "ABC12", claimedBy: "alice", expiresAt: 500 }],
  });
  const after = applyEvent(before, {
    id: 13,
    task_id: "ABC12",
    actor: "Jobs",
    event_type: "claim_expired",
    detail: { was_claimed_by: "alice", was_expires_at: 500 },
  });

  assert.equal(after.tasks.get("ABC12").status, "available");
  assert.equal(after.claims.has("ABC12"), false);
});

// --- reverse fold (forward-then-reverse identity) ---
//
// Each forward-foldable event type with breadcrumbs round-trips
// to identity. Pre-breadcrumb events (older DBs) are tested
// separately by asserting reverseEvent returns null.

const roundTripCases = [
  {
    name: "created",
    seed: () => emptyFrame(0),
    event: {
      id: 1,
      task_id: "ABC12",
      actor: "alice",
      event_type: "created",
      detail: {
        title: "T",
        description: "d",
        parent_id: "",
        sort_order: 1,
      },
    },
  },
  {
    name: "claimed (fresh)",
    seed: () => initialFrame({
      headEventId: 0,
      tasks: [{ shortId: "ABC12", title: "T", status: "available", sortOrder: 0 }],
      blocks: [],
      claims: [],
    }),
    event: {
      id: 2,
      task_id: "ABC12",
      actor: "alice",
      event_type: "claimed",
      detail: { duration: "30m", expires_at: 1500 },
    },
  },
  {
    name: "claimed (--force override)",
    seed: () => initialFrame({
      headEventId: 0,
      tasks: [{ shortId: "ABC12", title: "T", status: "claimed", sortOrder: 0 }],
      blocks: [],
      claims: [{ shortId: "ABC12", claimedBy: "alice", expiresAt: 800 }],
    }),
    event: {
      id: 3,
      task_id: "ABC12",
      actor: "bob",
      event_type: "claimed",
      detail: {
        duration: "30m",
        expires_at: 1500,
        was_claimed_by: "alice",
        was_expires_at: 800,
      },
    },
  },
  {
    name: "released (post-breadcrumb)",
    seed: () => initialFrame({
      headEventId: 0,
      tasks: [{ shortId: "ABC12", title: "T", status: "claimed", sortOrder: 0 }],
      blocks: [],
      claims: [{ shortId: "ABC12", claimedBy: "alice", expiresAt: 1500 }],
    }),
    event: {
      id: 4,
      task_id: "ABC12",
      actor: "alice",
      event_type: "released",
      detail: { was_claimed_by: "alice", was_expires_at: 1500 },
    },
  },
  {
    name: "done (was claimed)",
    seed: () => initialFrame({
      headEventId: 0,
      tasks: [{ shortId: "ABC12", title: "T", status: "claimed", sortOrder: 0 }],
      blocks: [],
      claims: [{ shortId: "ABC12", claimedBy: "alice", expiresAt: 1500 }],
    }),
    event: {
      id: 5,
      task_id: "ABC12",
      actor: "alice",
      event_type: "done",
      detail: {
        cascade: false,
        was_status: "claimed",
        was_claimed_by: "alice",
        was_expires_at: 1500,
      },
    },
  },
  {
    name: "done (was available)",
    seed: () => initialFrame({
      headEventId: 0,
      tasks: [{ shortId: "ABC12", title: "T", status: "available", sortOrder: 0 }],
      blocks: [],
      claims: [],
    }),
    event: {
      id: 6,
      task_id: "ABC12",
      actor: "alice",
      event_type: "done",
      detail: { cascade: false, was_status: "available" },
    },
  },
  {
    name: "canceled (was claimed)",
    seed: () => initialFrame({
      headEventId: 0,
      tasks: [{ shortId: "ABC12", title: "T", status: "claimed", sortOrder: 0 }],
      blocks: [],
      claims: [{ shortId: "ABC12", claimedBy: "alice", expiresAt: 1500 }],
    }),
    event: {
      id: 7,
      task_id: "ABC12",
      actor: "alice",
      event_type: "canceled",
      detail: {
        cascade: false,
        reason: "no",
        was_status: "claimed",
        was_claimed_by: "alice",
        was_expires_at: 1500,
      },
    },
  },
  {
    name: "claim_expired",
    seed: () => initialFrame({
      headEventId: 0,
      tasks: [{ shortId: "ABC12", title: "T", status: "claimed", sortOrder: 0 }],
      blocks: [],
      claims: [{ shortId: "ABC12", claimedBy: "alice", expiresAt: 500 }],
    }),
    event: {
      id: 8,
      task_id: "ABC12",
      actor: "Jobs",
      event_type: "claim_expired",
      detail: { was_claimed_by: "alice", was_expires_at: 500 },
    },
  },
  {
    name: "reopened (from done)",
    seed: () => initialFrame({
      headEventId: 0,
      tasks: [{ shortId: "ABC12", title: "T", status: "done", sortOrder: 0 }],
      blocks: [],
      claims: [],
    }),
    event: {
      id: 9,
      task_id: "ABC12",
      actor: "alice",
      event_type: "reopened",
      detail: { from_status: "done" },
    },
  },
  {
    name: "blocked",
    seed: () => initialFrame({
      headEventId: 0,
      tasks: [
        { shortId: "ABC12", title: "T", status: "available", sortOrder: 0 },
        { shortId: "XYZ99", title: "B", status: "available", sortOrder: 0 },
      ],
      blocks: [],
      claims: [],
    }),
    event: {
      id: 10,
      task_id: "ABC12",
      actor: "alice",
      event_type: "blocked",
      detail: { blocked_id: "ABC12", blocker_id: "XYZ99" },
    },
  },
  {
    name: "unblocked",
    seed: () => initialFrame({
      headEventId: 0,
      tasks: [
        { shortId: "ABC12", title: "T", status: "available", sortOrder: 0 },
        { shortId: "XYZ99", title: "B", status: "available", sortOrder: 0 },
      ],
      blocks: [{ blockedShortId: "ABC12", blockerShortId: "XYZ99" }],
      claims: [],
    }),
    event: {
      id: 11,
      task_id: "ABC12",
      actor: "alice",
      event_type: "unblocked",
      detail: {
        blocked_id: "ABC12",
        blocker_id: "XYZ99",
        reason: "blocker_done",
      },
    },
  },
  {
    name: "labeled",
    seed: () => initialFrame({
      headEventId: 0,
      tasks: [
        {
          shortId: "ABC12",
          title: "T",
          status: "available",
          sortOrder: 0,
          labels: ["existing"],
        },
      ],
      blocks: [],
      claims: [],
    }),
    event: {
      id: 12,
      task_id: "ABC12",
      actor: "alice",
      event_type: "labeled",
      detail: { names: ["web", "dx"], existing: ["existing"] },
    },
  },
  {
    name: "edited",
    seed: () => initialFrame({
      headEventId: 0,
      tasks: [
        {
          shortId: "ABC12",
          title: "old title",
          description: "old desc",
          status: "available",
          sortOrder: 0,
        },
      ],
      blocks: [],
      claims: [],
    }),
    event: {
      id: 13,
      task_id: "ABC12",
      actor: "alice",
      event_type: "edited",
      detail: {
        old_title: "old title",
        new_title: "new title",
        old_desc: "old desc",
        new_desc: "new desc",
      },
    },
  },
  {
    name: "moved",
    seed: () => initialFrame({
      headEventId: 0,
      tasks: [
        { shortId: "ABC12", title: "T", status: "available", sortOrder: 5 },
      ],
      blocks: [],
      claims: [],
    }),
    event: {
      id: 14,
      task_id: "ABC12",
      actor: "alice",
      event_type: "moved",
      detail: {
        direction: "after",
        relative_to: "XYZ99",
        old_sort_order: 5,
        new_sort_order: 10,
      },
    },
  },
];

for (const tc of roundTripCases) {
  test(`reverseEvent ${tc.name}: forward then reverse returns identity`, () => {
    const seed = tc.seed();
    // Round-trip semantics: forward(event with id=X) takes a frame at
    // event id X-1 to one at X; reverse(event id X) goes back. Align
    // the seed's cursor so the reverse lands on the same eventId.
    seed.eventId = tc.event.id - 1;
    const forwarded = applyEvent(seed, tc.event);
    const reversed = reverseEvent(forwarded, tc.event);
    assert.notEqual(reversed, null, "reverse should not be null for breadcrumb-bearing events");
    assertFramesEqual(reversed, seed, "round-trip should equal seed");
  });
}

// --- reverse-fold pre-breadcrumb fallback ---

test("reverseEvent done without was_status: returns null (caller falls back)", () => {
  const seed = initialFrame({
    headEventId: 0,
    tasks: [{ shortId: "ABC12", title: "T", status: "done", sortOrder: 0 }],
    blocks: [],
    claims: [],
  });
  const event = {
    id: 100,
    task_id: "ABC12",
    actor: "alice",
    event_type: "done",
    detail: { cascade: false }, // pre-breadcrumb event: no was_status
  };
  assert.equal(reverseEvent(seed, event), null);
});

test("reverseEvent released without was_claimed_by: returns null", () => {
  const seed = initialFrame({
    headEventId: 0,
    tasks: [{ shortId: "ABC12", title: "T", status: "available", sortOrder: 0 }],
    blocks: [],
    claims: [],
  });
  const event = {
    id: 101,
    task_id: "ABC12",
    actor: "alice",
    event_type: "released",
    detail: {}, // pre-breadcrumb
  };
  assert.equal(reverseEvent(seed, event), null);
});

// --- FrameCache ---

test("FrameCache: nearestAtOrBefore returns the largest frame <= target", () => {
  const cache = new FrameCache({ snapshotEvery: 50 });
  cache.set(emptyFrame(0));
  cache.set(emptyFrame(50));
  cache.set(emptyFrame(150));

  assert.equal(cache.nearestAtOrBefore(0).eventId, 0);
  assert.equal(cache.nearestAtOrBefore(49).eventId, 0);
  assert.equal(cache.nearestAtOrBefore(50).eventId, 50);
  assert.equal(cache.nearestAtOrBefore(149).eventId, 50);
  assert.equal(cache.nearestAtOrBefore(999).eventId, 150);
});

test("FrameCache: nearestAtOrAfter returns the smallest frame >= target", () => {
  const cache = new FrameCache({ snapshotEvery: 50 });
  cache.set(emptyFrame(50));
  cache.set(emptyFrame(150));

  assert.equal(cache.nearestAtOrAfter(0).eventId, 50);
  assert.equal(cache.nearestAtOrAfter(50).eventId, 50);
  assert.equal(cache.nearestAtOrAfter(51).eventId, 150);
  assert.equal(cache.nearestAtOrAfter(150).eventId, 150);
  assert.equal(cache.nearestAtOrAfter(151), null);
});

test("FrameCache: set is idempotent on event id", () => {
  const cache = new FrameCache({ snapshotEvery: 50 });
  const f1 = emptyFrame(50);
  cache.set(f1);
  cache.set(f1);
  assert.equal(cache.size(), 1);
});

test("FrameCache: shouldSnapshot fires every snapshotEvery events from anchor", () => {
  const cache = new FrameCache({ snapshotEvery: 10 });
  // From anchor=0: snapshot at 10, 20, 30, ...
  assert.equal(cache.shouldSnapshot(5, 0), false);
  assert.equal(cache.shouldSnapshot(10, 0), true);
  assert.equal(cache.shouldSnapshot(20, 0), true);
  assert.equal(cache.shouldSnapshot(25, 0), false);
});
