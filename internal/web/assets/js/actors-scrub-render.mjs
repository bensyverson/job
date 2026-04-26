/*
  Actors-board HTML emitter for the scrubber.

  Pure string functions mirroring internal/web/templates/html/pages/
  actors.html.tmpl. Driver parses the output with DOMParser and swaps
  the resulting <div data-actors-board> into the live page so the
  data-* hooks the live updater (actors-live.js) and progressive-
  enhancement (data-peek) keep working uniformly across SSR and
  history mode.
*/

import { escapeHTML } from "./scrub-util.mjs";

function renderCard(c) {
  const claimAttr = c.isClaim ? ` data-claim="1"` : "";
  const notes =
    c.noteText && c.noteCount > 0
      ? `<span class="c-actor-card__notes" data-note-count="${c.noteCount}">${escapeHTML(c.noteText)}</span>`
      : "";
  const desc =
    c.taskDesc && c.taskDesc !== ""
      ? `<p class="c-actor-card__desc">${escapeHTML(c.taskDesc)}</p>`
      : "";
  return (
    `<article class="c-actor-card ${escapeHTML(c.stateClass)}" data-actor-task="${escapeHTML(c.cardKey)}" data-event-at="${c.eventAt}"${claimAttr}>` +
    `<div class="c-actor-card__meta">` +
    `<span class="c-log-row__verb ${escapeHTML(c.verbClass)}">${escapeHTML(c.verb)}</span>` +
    notes +
    `<time>${escapeHTML(c.ageText)}</time>` +
    `</div>` +
    `<div class="c-actor-card__title-row">` +
    `<span class="c-id-pill">${escapeHTML(c.taskShortID)}</span>` +
    `<h4 class="c-actor-card__title">${escapeHTML(c.taskTitle)}</h4>` +
    `</div>` +
    desc +
    `<a href="${escapeHTML(c.taskURL)}" data-peek class="c-row-link" aria-label="Open task ${escapeHTML(c.taskShortID)}"></a>` +
    `</article>`
  );
}

function renderColumn(col) {
  const idle = col.idle ? " c-actor-col--idle" : "";
  const idleStatus = col.idle ? " c-actor-col__status--idle" : "";
  const cards = (col.cards ?? []).map(renderCard).join("");
  return (
    `<section class="c-actor-col${idle}" aria-labelledby="a-${escapeHTML(col.name)}" data-actor="${escapeHTML(col.name)}">` +
    `<header class="c-actor-col__header">` +
    `<a href="${escapeHTML(col.url)}" class="c-avatar c-avatar-lg" data-actor="${escapeHTML(col.name)}" aria-label="Open ${escapeHTML(col.name)}"></a>` +
    `<div class="stack stack-gap-xs" style="min-width: 0">` +
    `<a id="a-${escapeHTML(col.name)}" href="${escapeHTML(col.url)}" class="c-actor-col__name" data-actor="${escapeHTML(col.name)}">${escapeHTML(col.name)}</a>` +
    `<span class="c-actor-col__status${idleStatus}">${escapeHTML(col.statusText)}</span>` +
    `</div>` +
    `</header>` +
    `<div class="c-actor-col__stream">${cards}</div>` +
    `</section>`
  );
}

export function renderActorsBoard(cols) {
  const inner = (cols ?? []).map(renderColumn).join("");
  return `<div class="c-actors-board" aria-label="Actors" data-actors-board>${inner}</div>`;
}
