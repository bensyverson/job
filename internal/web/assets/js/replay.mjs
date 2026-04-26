/*
  Replay buffer for the dashboard's time-travel scrubber.

  Maintains a frame cache that lets the scrubber answer "what did the
  world look like at event N?" without re-reading the full event log
  on every tick. The frame cache is doubly linked: applyEvent applies
  one event's delta forward; reverseEvent uses the prior-state
  breadcrumbs (was_status, was_claimed_by, was_expires_at, plus the
  per-event old_title, old_desc, old_sort_order, from_status, and
  existing-labels payload fields the server already carries) to undo
  a delta.

  Public API (scoped to what the scrubber UI and timeline need):

    initialFrame({ headEventId, tasks, blocks, claims }) -> Frame
        Build the frame the SSR layer hydrated us with — the "head"
        the cache pins as the live state.

    applyEvent(frame, event) -> Frame
        Pure forward fold. Returns a new frame with the event applied.

    reverseEvent(frame, event) -> Frame | null
        Pure reverse fold. Returns null when the breadcrumbs needed to
        undo the event are missing (pre-breadcrumb events, described
        in commit 915916d). Callers fall back to forward replay from a
        snapshot.

    FrameCache({ snapshotEvery })
        Stores frames by event id. nearestAtOrBefore(target) returns
        the largest cached frame <= target; nearestAtOrAfter(target)
        returns the smallest cached frame >= target. set(frame) is
        idempotent on event id; size() reports cache fill;
        shouldSnapshot(eventId, anchor) is the cadence helper used by
        replay loops to decide when to checkpoint.

  Out of scope here: HTTP fetching from /events, view-specific DOM
  updates, the scrubber pill UI. Those land in FxhFR / Xg742 and the
  per-view *-live.mjs modules. This module is pure data.
*/

// Frame shape:
//   {
//     eventId: number,
//     tasks: Map<shortId, TaskState>,
//     blocks: Map<blockedShortId, Set<blockerShortId>>,
//     claims: Map<shortId, { claimedBy, expiresAt }>,
//   }

function defaultTask(shortId) {
  return {
    shortId,
    title: "",
    description: "",
    status: "available",
    parentShortId: null,
    sortOrder: 0,
    labels: new Set(),
  };
}

function cloneFrame(frame) {
  const tasks = new Map();
  for (const [k, v] of frame.tasks) {
    tasks.set(k, { ...v, labels: new Set(v.labels) });
  }
  const blocks = new Map();
  for (const [k, set] of frame.blocks) {
    blocks.set(k, new Set(set));
  }
  return {
    eventId: frame.eventId,
    tasks,
    blocks,
    claims: new Map(frame.claims),
  };
}

export function initialFrame(payload) {
  const tasks = new Map();
  for (const t of payload.tasks ?? []) {
    tasks.set(t.shortId, {
      ...defaultTask(t.shortId),
      ...t,
      parentShortId: t.parentShortId ?? null,
      labels: new Set(t.labels ?? []),
    });
  }
  const blocks = new Map();
  for (const b of payload.blocks ?? []) {
    let set = blocks.get(b.blockedShortId);
    if (!set) {
      set = new Set();
      blocks.set(b.blockedShortId, set);
    }
    set.add(b.blockerShortId);
  }
  const claims = new Map();
  for (const c of payload.claims ?? []) {
    claims.set(c.shortId, { claimedBy: c.claimedBy, expiresAt: c.expiresAt });
  }
  return {
    eventId: payload.headEventId ?? 0,
    tasks,
    blocks,
    claims,
  };
}

// applyEvent dispatch — one tiny helper per event type. Each helper
// receives a *cloned* frame it may mutate, plus the event envelope.
const FORWARD = {
  created(frame, event) {
    const detail = event.detail ?? {};
    frame.tasks.set(event.task_id, {
      ...defaultTask(event.task_id),
      title: detail.title ?? "",
      description: detail.description ?? "",
      parentShortId: detail.parent_id ? detail.parent_id : null,
      sortOrder: detail.sort_order ?? 0,
      status: "available",
      labels: new Set(),
    });
  },

  claimed(frame, event) {
    const detail = event.detail ?? {};
    const t = frame.tasks.get(event.task_id);
    if (t) t.status = "claimed";
    frame.claims.set(event.task_id, {
      claimedBy: event.actor,
      expiresAt: detail.expires_at ?? 0,
    });
  },

  released(frame, event) {
    const t = frame.tasks.get(event.task_id);
    if (t) t.status = "available";
    frame.claims.delete(event.task_id);
  },

  done(frame, event) {
    const t = frame.tasks.get(event.task_id);
    if (t) t.status = "done";
    frame.claims.delete(event.task_id);
  },

  canceled(frame, event) {
    const t = frame.tasks.get(event.task_id);
    if (t) t.status = "canceled";
    frame.claims.delete(event.task_id);
  },

  reopened(frame, event) {
    const t = frame.tasks.get(event.task_id);
    if (t) t.status = "available";
  },

  blocked(frame, event) {
    const detail = event.detail ?? {};
    const blocked = detail.blocked_id;
    const blocker = detail.blocker_id;
    if (!blocked || !blocker) return;
    let set = frame.blocks.get(blocked);
    if (!set) {
      set = new Set();
      frame.blocks.set(blocked, set);
    }
    set.add(blocker);
  },

  unblocked(frame, event) {
    const detail = event.detail ?? {};
    const blocked = detail.blocked_id;
    const blocker = detail.blocker_id;
    if (!blocked || !blocker) return;
    const set = frame.blocks.get(blocked);
    if (!set) return;
    set.delete(blocker);
    if (set.size === 0) frame.blocks.delete(blocked);
  },

  labeled(frame, event) {
    const detail = event.detail ?? {};
    const t = frame.tasks.get(event.task_id);
    if (!t) return;
    for (const name of detail.names ?? []) t.labels.add(name);
  },

  edited(frame, event) {
    const detail = event.detail ?? {};
    const t = frame.tasks.get(event.task_id);
    if (!t) return;
    if (detail.new_title !== undefined) t.title = detail.new_title;
    if (detail.new_desc !== undefined) t.description = detail.new_desc;
  },

  moved(frame, event) {
    const detail = event.detail ?? {};
    const t = frame.tasks.get(event.task_id);
    if (!t) return;
    if (detail.new_sort_order !== undefined) t.sortOrder = detail.new_sort_order;
  },

  noted(frame, event) {
    const detail = event.detail ?? {};
    const t = frame.tasks.get(event.task_id);
    if (!t) return;
    if (detail.description_after !== undefined) {
      t.description = detail.description_after;
    }
  },

  claim_expired(frame, event) {
    const t = frame.tasks.get(event.task_id);
    if (t) t.status = "available";
    frame.claims.delete(event.task_id);
  },
};

export function applyEvent(frame, event) {
  const next = cloneFrame(frame);
  const handler = FORWARD[event.event_type];
  if (handler) handler(next, event);
  next.eventId = event.id;
  return next;
}

// Reverse helpers. Each returns true on success or false when the
// payload is missing the breadcrumbs needed to invert. reverseEvent
// short-circuits to null on first false.
const REVERSE = {
  created(frame, event) {
    frame.tasks.delete(event.task_id);
    return true;
  },

  claimed(frame, event) {
    const detail = event.detail ?? {};
    if (detail.was_claimed_by !== undefined) {
      // Override path: restore the displaced holder.
      frame.claims.set(event.task_id, {
        claimedBy: detail.was_claimed_by,
        expiresAt: detail.was_expires_at ?? 0,
      });
      const t = frame.tasks.get(event.task_id);
      if (t) t.status = "claimed";
    } else {
      // Fresh claim: simply remove and revert task to available.
      frame.claims.delete(event.task_id);
      const t = frame.tasks.get(event.task_id);
      if (t) t.status = "available";
    }
    return true;
  },

  released(frame, event) {
    const detail = event.detail ?? {};
    if (detail.was_claimed_by === undefined) return false;
    frame.claims.set(event.task_id, {
      claimedBy: detail.was_claimed_by,
      expiresAt: detail.was_expires_at ?? 0,
    });
    const t = frame.tasks.get(event.task_id);
    if (t) t.status = "claimed";
    return true;
  },

  done(frame, event) {
    const detail = event.detail ?? {};
    if (detail.was_status === undefined) return false;
    const t = frame.tasks.get(event.task_id);
    if (t) t.status = detail.was_status;
    if (detail.was_status === "claimed" && detail.was_claimed_by !== undefined) {
      frame.claims.set(event.task_id, {
        claimedBy: detail.was_claimed_by,
        expiresAt: detail.was_expires_at ?? 0,
      });
    }
    return true;
  },

  canceled(frame, event) {
    return REVERSE.done(frame, event);
  },

  reopened(frame, event) {
    const detail = event.detail ?? {};
    if (detail.from_status === undefined) return false;
    const t = frame.tasks.get(event.task_id);
    if (t) t.status = detail.from_status;
    return true;
  },

  blocked(frame, event) {
    const detail = event.detail ?? {};
    const blocked = detail.blocked_id;
    const blocker = detail.blocker_id;
    if (!blocked || !blocker) return false;
    const set = frame.blocks.get(blocked);
    if (set) {
      set.delete(blocker);
      if (set.size === 0) frame.blocks.delete(blocked);
    }
    return true;
  },

  unblocked(frame, event) {
    const detail = event.detail ?? {};
    const blocked = detail.blocked_id;
    const blocker = detail.blocker_id;
    if (!blocked || !blocker) return false;
    let set = frame.blocks.get(blocked);
    if (!set) {
      set = new Set();
      frame.blocks.set(blocked, set);
    }
    set.add(blocker);
    return true;
  },

  labeled(frame, event) {
    const detail = event.detail ?? {};
    const t = frame.tasks.get(event.task_id);
    if (!t) return true;
    const existing = new Set(detail.existing ?? []);
    for (const name of detail.names ?? []) {
      if (!existing.has(name)) t.labels.delete(name);
    }
    return true;
  },

  edited(frame, event) {
    const detail = event.detail ?? {};
    const t = frame.tasks.get(event.task_id);
    if (!t) return true;
    if (detail.new_title !== undefined && detail.old_title === undefined) return false;
    if (detail.new_desc !== undefined && detail.old_desc === undefined) return false;
    if (detail.old_title !== undefined) t.title = detail.old_title;
    if (detail.old_desc !== undefined) t.description = detail.old_desc;
    return true;
  },

  moved(frame, event) {
    const detail = event.detail ?? {};
    const t = frame.tasks.get(event.task_id);
    if (!t) return true;
    if (detail.old_sort_order === undefined) return false;
    t.sortOrder = detail.old_sort_order;
    return true;
  },

  noted(_frame, _event) {
    // noted carries description_after but no description_before.
    // Reverse-fold isn't exact for the description field. Caller
    // falls back to forward replay from a snapshot.
    return false;
  },

  claim_expired(frame, event) {
    const detail = event.detail ?? {};
    if (detail.was_claimed_by === undefined) return false;
    frame.claims.set(event.task_id, {
      claimedBy: detail.was_claimed_by,
      expiresAt: detail.was_expires_at ?? 0,
    });
    const t = frame.tasks.get(event.task_id);
    if (t) t.status = "claimed";
    return true;
  },
};

export function reverseEvent(frame, event) {
  const handler = REVERSE[event.event_type];
  if (!handler) return null;
  const next = cloneFrame(frame);
  next.eventId = event.id - 1;
  if (!handler(next, event)) return null;
  return next;
}

// FrameCache stores frames by event id and answers "nearest snapshot"
// queries. The cache is a Map plus a sorted index of event ids; both
// are kept in sync. Insertions are O(log n) via binary search; lookups
// are O(log n). Cache eviction is intentionally not implemented in v1
// — the dashboard's event volume in practice is low enough that the
// memory footprint is bounded by the head event id divided by
// snapshotEvery, which is fine for a local-first tool.
export class FrameCache {
  constructor(opts = {}) {
    this.snapshotEvery = opts.snapshotEvery ?? 50;
    this.frames = new Map(); // eventId -> Frame
    this.sortedIds = [];     // sorted ascending
  }

  set(frame) {
    if (this.frames.has(frame.eventId)) return;
    this.frames.set(frame.eventId, frame);
    // Insertion-sort into sortedIds.
    const id = frame.eventId;
    let lo = 0;
    let hi = this.sortedIds.length;
    while (lo < hi) {
      const mid = (lo + hi) >>> 1;
      if (this.sortedIds[mid] < id) lo = mid + 1;
      else hi = mid;
    }
    this.sortedIds.splice(lo, 0, id);
  }

  size() {
    return this.frames.size;
  }

  // nearestAtOrBefore(target) returns the cached frame with the
  // largest event id <= target, or null if no such frame exists.
  nearestAtOrBefore(target) {
    let lo = 0;
    let hi = this.sortedIds.length;
    while (lo < hi) {
      const mid = (lo + hi) >>> 1;
      if (this.sortedIds[mid] <= target) lo = mid + 1;
      else hi = mid;
    }
    // After the loop, lo is one past the last id <= target.
    if (lo === 0) return null;
    return this.frames.get(this.sortedIds[lo - 1]);
  }

  // nearestAtOrAfter(target) returns the cached frame with the
  // smallest event id >= target, or null if no such frame exists.
  nearestAtOrAfter(target) {
    let lo = 0;
    let hi = this.sortedIds.length;
    while (lo < hi) {
      const mid = (lo + hi) >>> 1;
      if (this.sortedIds[mid] < target) lo = mid + 1;
      else hi = mid;
    }
    if (lo === this.sortedIds.length) return null;
    return this.frames.get(this.sortedIds[lo]);
  }

  // shouldSnapshot decides whether a replay loop should checkpoint
  // after applying the event with id `eventId`, given that the loop
  // started from an anchor with id `anchor`. Snapshots fire every
  // snapshotEvery events from the anchor.
  shouldSnapshot(eventId, anchor) {
    if (eventId === anchor) return false;
    return (eventId - anchor) % this.snapshotEvery === 0;
  }
}
