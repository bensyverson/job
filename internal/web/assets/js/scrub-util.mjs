/*
  Shared utilities for the per-view scrubber modules.

  Two helpers that every view's history-mode renderer needs:

    relativeTime(nowSec, thenSec)
        Formats a unix-second delta as a compact label matching the Go
        render.RelativeTime ladder ("just now", "5s", "2m", "1h 5m",
        "3d 2h"). Future timestamps render non-negative.

    escapeHTML(s)
        Escapes the five HTML-significant characters into numeric
        entities. Matches Go html/template's default closely enough to
        keep titles, descriptions, and note bodies safe in both
        attribute and text positions.
*/

export function relativeTime(nowSec, thenSec) {
  let d = nowSec - thenSec;
  if (d < 0) d = -d;
  if (d < 1) return "just now";
  if (d < 60) return `${Math.floor(d)}s`;
  if (d < 3600) return `${Math.floor(d / 60)}m`;
  if (d < 86400) {
    const h = Math.floor(d / 3600);
    const m = Math.floor((d - h * 3600) / 60);
    return m === 0 ? `${h}h` : `${h}h ${m}m`;
  }
  const days = Math.floor(d / 86400);
  const remH = Math.floor((d - days * 86400) / 3600);
  return remH === 0 ? `${days}d` : `${days}d ${remH}h`;
}

export function escapeHTML(s) {
  if (s == null) return "";
  return String(s)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&#34;")
    .replace(/'/g, "&#39;");
}
