/*
  Home-view HTML emitter for the scrubber.

  Pure string functions mirroring internal/web/templates/html/pages/
  home.html.tmpl for the four signal cards + four panels. The driver
  parses each fragment with DOMParser and swaps the live <div
  class="c-grid-signals"> and the four [data-home-*] sections in place,
  preserving the page chrome (header, footer, peek-sheet, scrubber pill,
  dependency-flow graph) untouched.

  The graph is intentionally NOT rendered here — it's refetched server-
  side via POST /home/graph (debounced) so the JS bundle doesn't need
  to carry the subway layout pipeline.
*/

import { escapeHTML } from "./scrub-util.mjs";

// --- Signal cards (4 across the c-grid-signals row) ---

const ACTIVITY_ICON =
  '<svg class="c-signal-card__icon" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true">' +
  '<rect x="2" y="9" width="2.25" height="5" rx="0.5" />' +
  '<rect x="6.875" y="5" width="2.25" height="9" rx="0.5" />' +
  '<rect x="11.75" y="2" width="2.25" height="12" rx="0.5" />' +
  "</svg>";

const NEWLY_BLOCKED_ICON =
  '<svg class="c-signal-card__icon" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true">' +
  '<rect x="2.5" y="2.5" width="11" height="11" rx="2" />' +
  '<path d="M5 5l6 6M11 5l-6 6" stroke-linecap="round" />' +
  "</svg>";

const LONGEST_CLAIM_ICON =
  '<svg class="c-signal-card__icon" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true">' +
  '<circle cx="8" cy="8" r="6" />' +
  '<path d="M8 4.5v3.5h3" stroke-linecap="round" />' +
  "</svg>";

const OLDEST_TODO_ICON =
  '<svg class="c-signal-card__icon" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true">' +
  '<path d="M8 2v12M2 8h12" stroke-linecap="round" />' +
  "</svg>";

function renderActivityCard(a) {
  const bars = a.Bars.map((b) => {
    if (b.Empty) return '<span class="c-histogram__bar c-histogram__bar--empty"></span>';
    const segs = [];
    if (b.Done > 0) segs.push(`<i class="c-histogram__seg c-histogram__seg--done" style="flex:${b.Done}"></i>`);
    if (b.Claim > 0) segs.push(`<i class="c-histogram__seg c-histogram__seg--claim" style="flex:${b.Claim}"></i>`);
    if (b.Create > 0) segs.push(`<i class="c-histogram__seg c-histogram__seg--create" style="flex:${b.Create}"></i>`);
    if (b.Block > 0) segs.push(`<i class="c-histogram__seg c-histogram__seg--block" style="flex:${b.Block}"></i>`);
    return `<span class="c-histogram__bar" style="--h:${b.HeightPercent}%">${segs.join("")}</span>`;
  }).join("");
  return (
    '<article class="c-signal-card c-signal-card--primary">' +
    '<div class="c-signal-card__header">' +
    ACTIVITY_ICON +
    '<span class="c-signal-card__label">Activity · last 60m</span>' +
    "</div>" +
    `<div class="c-histogram" role="img" aria-label="${a.TotalEvents} events in the last hour, stacked by event type">${bars}</div>` +
    '<div class="c-signal-card__context c-hist-legend">' +
    `<span class="c-hist-legend__item"><span class="c-hist-swatch c-hist-swatch--done"></span>${a.TotalDone} done</span>` +
    `<span class="c-hist-legend__item"><span class="c-hist-swatch c-hist-swatch--claim"></span>${a.TotalClaim} claimed</span>` +
    `<span class="c-hist-legend__item"><span class="c-hist-swatch c-hist-swatch--create"></span>${a.TotalCreate} new</span>` +
    `<span class="c-hist-legend__item"><span class="c-hist-swatch c-hist-swatch--block"></span>${a.TotalBlock} blocked</span>` +
    "</div>" +
    '<div class="c-signal-card__underline"></div>' +
    "</article>"
  );
}

function renderNewlyBlockedCard(nb) {
  let context;
  if (nb.Items && nb.Items.length > 0) {
    const first = nb.Items[0];
    context =
      `<a href="${escapeHTML(first.BlockedURL)}" class="c-id-pill">${escapeHTML(first.BlockedShortID)}</a> waiting on ` +
      `<a href="${escapeHTML(first.WaitingOnURL)}" class="c-id-pill">${escapeHTML(first.WaitingOnShortID)}</a>`;
  } else {
    context = '<span class="t-muted">No new blocks</span>';
  }
  return (
    `<article class="c-signal-card c-signal-card--warn" style="--progress: ${nb.ProgressPct}%">` +
    '<div class="c-signal-card__header">' +
    NEWLY_BLOCKED_ICON +
    '<span class="c-signal-card__label">Newly blocked · 10m</span>' +
    "</div>" +
    `<div class="c-signal-card__value">${nb.Count}</div>` +
    `<div class="c-signal-card__context">${context}</div>` +
    '<div class="c-signal-card__underline"></div>' +
    "</article>"
  );
}

function renderLongestClaimCard(lc) {
  let value, context;
  if (lc.Present) {
    value = `<div class="c-signal-card__value">${escapeHTML(lc.DurationText)}</div>`;
    context =
      '<div class="c-signal-card__context">' +
      `<span class="c-avatar c-avatar-dot" data-actor="${escapeHTML(lc.Actor)}"></span>` +
      `<a href="${escapeHTML(lc.ActorURL)}"><strong class="t-muted">${escapeHTML(lc.Actor)}</strong></a> · ` +
      `<a href="${escapeHTML(lc.TaskURL)}" class="c-id-pill">${escapeHTML(lc.TaskShortID)}</a>` +
      "</div>";
  } else {
    value = '<div class="c-signal-card__value">—</div>';
    context = '<div class="c-signal-card__context"><span class="t-muted">No active claims</span></div>';
  }
  return (
    `<article class="c-signal-card c-signal-card--primary" style="--progress: ${lc.ProgressPct}%">` +
    '<div class="c-signal-card__header">' +
    LONGEST_CLAIM_ICON +
    '<span class="c-signal-card__label">Longest active claim</span>' +
    "</div>" +
    value +
    context +
    '<div class="c-signal-card__underline"></div>' +
    "</article>"
  );
}

function renderOldestTodoCard(ot) {
  let value, context;
  if (ot.Present) {
    value = `<div class="c-signal-card__value">${escapeHTML(ot.AgeText)}</div>`;
    context =
      '<div class="c-signal-card__context">' +
      `<a href="${escapeHTML(ot.TaskURL)}" class="c-id-pill">${escapeHTML(ot.TaskShortID)}</a> — ${escapeHTML(ot.Title)}` +
      "</div>";
  } else {
    value = '<div class="c-signal-card__value">—</div>';
    context = '<div class="c-signal-card__context"><span class="t-muted">Nothing waiting</span></div>';
  }
  return (
    `<article class="c-signal-card c-signal-card--warn" style="--progress: ${ot.ProgressPct}%">` +
    '<div class="c-signal-card__header">' +
    OLDEST_TODO_ICON +
    '<span class="c-signal-card__label">Oldest todo</span>' +
    "</div>" +
    value +
    context +
    '<div class="c-signal-card__underline"></div>' +
    "</article>"
  );
}

export function renderSignals(bag) {
  return (
    '<div class="c-grid-signals">' +
    renderActivityCard(bag.Activity) +
    renderNewlyBlockedCard(bag.NewlyBlocked) +
    renderLongestClaimCard(bag.LongestClaim) +
    renderOldestTodoCard(bag.OldestTodo) +
    "</div>"
  );
}

// --- Panels (four data-home-* sections in c-grid-cols-4) ---

function emptyPanelRow(text) {
  return (
    '<div class="c-panel-row" style="--row-cols: 1fr">' +
    `<span class="c-panel-row__title t-muted">${escapeHTML(text)}</span>` +
    "</div>"
  );
}

export function renderActiveClaims(panel) {
  let body;
  if (panel.Rows && panel.Rows.length > 0) {
    body = panel.Rows.map(
      (r) =>
        '<div class="c-panel-row" style="--row-cols: var(--avatar-sm-size) 80px 1fr auto" ' +
        `data-claimed-at="${r.ClaimedAtUnix}">` +
        `<a href="${escapeHTML(r.ActorURL)}" class="c-avatar c-avatar-sm" data-actor="${escapeHTML(r.Actor)}" aria-label="Actor ${escapeHTML(r.Actor)}"></a>` +
        `<span class="c-id-pill">${escapeHTML(r.TaskShortID)}</span>` +
        `<span class="c-panel-row__title">${escapeHTML(r.TaskTitle)}</span>` +
        `<span class="c-panel-row__meta" data-claim-idle>${escapeHTML(r.DurationText)}</span>` +
        `<a href="${escapeHTML(r.TaskURL)}" data-peek class="c-row-link" aria-label="Open task ${escapeHTML(r.TaskShortID)}"></a>` +
        "</div>",
    ).join("");
  } else {
    body = emptyPanelRow("No active claims");
  }
  return (
    '<section class="c-panel" aria-labelledby="p-claims" data-home-claims>' +
    '<div class="c-panel__header">' +
    '<h2 id="p-claims" class="c-panel__title">Active claims</h2>' +
    `<span class="c-panel__meta">${panel.Count} in flight</span>` +
    "</div>" +
    `<div class="c-panel__list">${body}</div>` +
    "</section>"
  );
}

export function renderRecentCompletions(panel) {
  let body;
  if (panel.Rows && panel.Rows.length > 0) {
    body = panel.Rows.map(
      (r) =>
        '<div class="c-panel-row" style="--row-cols: var(--avatar-sm-size) 80px 1fr auto">' +
        `<a href="${escapeHTML(r.ActorURL)}" class="c-avatar c-avatar-sm" data-actor="${escapeHTML(r.Actor)}" aria-label="Actor ${escapeHTML(r.Actor)}"></a>` +
        `<span class="c-id-pill">${escapeHTML(r.TaskShortID)}</span>` +
        `<span class="c-panel-row__title">${escapeHTML(r.TaskTitle)}</span>` +
        `<span class="c-panel-row__meta">${escapeHTML(r.AgeText)}</span>` +
        `<a href="${escapeHTML(r.TaskURL)}" data-peek class="c-row-link" aria-label="Open task ${escapeHTML(r.TaskShortID)}"></a>` +
        "</div>",
    ).join("");
  } else {
    body = emptyPanelRow("No recent completions");
  }
  return (
    '<section class="c-panel" aria-labelledby="p-recent" data-home-recent>' +
    '<div class="c-panel__header">' +
    '<h2 id="p-recent" class="c-panel__title">Recent completions</h2>' +
    `<span class="c-panel__meta">last ${panel.Count}</span>` +
    "</div>" +
    `<div class="c-panel__list">${body}</div>` +
    "</section>"
  );
}

export function renderUpcoming(panel) {
  let body;
  if (panel.Rows && panel.Rows.length > 0) {
    body = panel.Rows.map(
      (r) =>
        `<div class="c-panel-row" style="--row-cols: 80px 1fr auto" data-created-at="${r.CreatedAtUnix}">` +
        `<span class="c-id-pill">${escapeHTML(r.TaskShortID)}</span>` +
        `<span class="c-panel-row__title">${escapeHTML(r.TaskTitle)}</span>` +
        `<span class="c-panel-row__meta">${escapeHTML(r.AgeText)}</span>` +
        `<a href="${escapeHTML(r.TaskURL)}" data-peek class="c-row-link" aria-label="Open task ${escapeHTML(r.TaskShortID)}"></a>` +
        "</div>",
    ).join("");
  } else {
    body = emptyPanelRow("No available tasks");
  }
  return (
    '<section class="c-panel" aria-labelledby="p-upcoming" data-home-upcoming>' +
    '<div class="c-panel__header">' +
    '<h2 id="p-upcoming" class="c-panel__title">Available</h2>' +
    `<span class="c-panel__meta">${panel.Count} ready</span>` +
    "</div>" +
    `<div class="c-panel__list">${body}</div>` +
    "</section>"
  );
}

export function renderBlocked(panel) {
  let body;
  if (panel.Rows && panel.Rows.length > 0) {
    body = panel.Rows.map((r) => {
      const blockerPills = r.Blockers.map(
        (b, i) =>
          (i > 0 ? ", " : "") +
          `<a href="${escapeHTML(b.URL)}" class="c-id-pill">${escapeHTML(b.ShortID)}</a>`,
      ).join("");
      return (
        '<div class="c-panel-row c-panel-row--stacked" style="--row-cols: 80px 1fr">' +
        `<span class="c-id-pill">${escapeHTML(r.TaskShortID)}</span>` +
        '<div class="stack stack-gap-xs">' +
        `<span class="c-panel-row__title">${escapeHTML(r.TaskTitle)}</span>` +
        `<span class="c-panel-row__meta">waiting on ${blockerPills}</span>` +
        "</div>" +
        `<a href="${escapeHTML(r.TaskURL)}" data-peek class="c-row-link" aria-label="Open task ${escapeHTML(r.TaskShortID)}"></a>` +
        "</div>"
      );
    }).join("");
  } else {
    body = emptyPanelRow("No blocked tasks");
  }
  return (
    '<section class="c-panel" aria-labelledby="p-blocked" data-home-blocked>' +
    '<div class="c-panel__header">' +
    '<h2 id="p-blocked" class="c-panel__title">Blocked</h2>' +
    `<span class="c-panel__meta">${panel.Count} waiting</span>` +
    "</div>" +
    `<div class="c-panel__list">${body}</div>` +
    "</section>"
  );
}
