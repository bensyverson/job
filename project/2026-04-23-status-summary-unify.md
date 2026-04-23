# Unifying `status` and `summary`

*Pivot from the cancelled P6 (ccWfW) during the 2026-04-23 CLI papercuts session.*

`status` and `summary` are structurally identical — aggregate-rollup plus
per-child block — and differ only in scope (forest vs subtree) and in
whether a session preamble prefixes the output. The right move is to
consolidate under a single shared renderer, keep `status` as the verb
(warmer than `summary`), and retire `summary` as a deprecated alias.

This plan replaces cancelled P6 with a wider set of subtasks that also
fold in tweaks we want on the rollup line itself (zero-count suppression,
closure timestamp for fully-done scopes, dropping redundant `: status`
tails in favor of exception-only reporting), a leaf-only collapse, and
two new signal lines (`Next:` and `Stale:`) that matter equally at the
forest and subtree levels.

```yaml
tasks:
  - title: Unify `status` and `summary` under `status`
    ref: unify
    desc: |
      Replacement for cancelled P6 (ccWfW). `status` and `summary` are
      structurally identical — aggregate-rollup-plus-per-child — and
      differ only in scope (forest vs subtree) and in session preamble.
      Consolidate under a single shared renderer; the `status` verb
      becomes the unified entry point; `summary` retires as a
      deprecated alias.
    labels: [cli, status, summary, design]
    children:

      - title: Extract shared subtree-rollup renderer
        ref: u1
        desc: |
          Pull summary's rollup+per-child logic into a renderer that
          takes (scope, opts) and works for both forest scope (all
          roots) and subtree scope (one parent). Behavior-preserving
          refactor at this stage: existing `summary <id>` continues to
          call it, and every existing summary test still passes
          unchanged. This task is purely mechanical — no output shape
          changes here; those land in u2 / u3.
        labels: [cli, refactor]

      - title: Tighten the rollup line
        ref: u2
        desc: |
          Three refinements to the shared renderer's headline and
          per-child block:

          (1) Suppress zero-count status tokens. Render only non-zero
              states. "10 of 10 done · 0 blocked · 0 available · 0 in
              flight" collapses to "10 of 10 done".

          (2) When the scope is fully complete (every descendant in
              done or canceled), append a closure timestamp to the
              rollup: "closed 2026-04-22 18:47". Source: latest
              done/canceled event within scope.

          (3) In the per-child block, drop the trailing ": <status>"
              when the child's status matches the scope's dominant
              status (done when the scope is mostly-done; otherwise
              available). Surface only exceptions — claimed,
              canceled, blocked — so the eye lands on what deviates.
        labels: [cli, status, ergonomics]
        blockedBy: [u1]

      - title: Leaf-only collapse with in-flight-only fallback
        ref: u3
        desc: |
          When the scope's direct children are all leaves (no
          descendants of their own), skip the per-child block entirely
          — the rollup already says everything worth saying.

          Exception: if any of those leaves is currently claimed,
          list ONLY those claimed rows, suppressing the
          done/available/blocked/canceled noise. Gives the
          session-start "who's working on what" signal without the
          bulk of the full block. If zero claimed, zero rows.
        labels: [cli, status, ergonomics]
        blockedBy: [u1]

      - title: status (no-arg) renders top-level forest via shared renderer
        ref: u4
        desc: |
          Rewrite `status` without arguments to keep its session
          preamble (the open/done/canceled tally line, the identity
          line) and delegate the body to the shared renderer with
          forest scope.

          Output shape:
            <preamble line 1: counts + last activity>
            <preamble line 2: identity>
            <blank line>
            <shared renderer — one row per top-level task with its own
             rollup, per the u1/u2/u3 shape>
        labels: [cli, status]
        blockedBy: [u1]

      - title: status <id> delegates to shared renderer
        ref: u5
        desc: |
          Add a `status <id>` form that scopes the shared renderer to
          the subtree rooted at <id>. Behavior is what `summary <id>`
          does today, plus the u2/u3 tightenings and u6/u7 additions
          once they land. No session preamble at the node level — the
          preamble is DB-wide metadata and doesn't belong on a subtree
          view.
        labels: [cli, status]
        blockedBy: [u1]

      - title: "Surface Next: in status output (forest + subtree)"
        ref: u6
        desc: |
          After the rollup and per-child block, append:
            Next: <id> "<title>"
          naming the first claimable leaf (unclaimed, unblocked,
          leaf-frontier) within scope. Uses the same
          queryAvailableLeafFrontier path as the P5 done-ack fallback
          (jqkTc). Omit the line when scope has no claimable leaves
          (fully done, or fully blocked). Forest scope: global
          frontier; subtree scope: frontier under <id>.
        labels: [cli, status, next]
        blockedBy: [u4, u5]

      - title: "Surface stale claims in status output (forest + subtree)"
        ref: u7
        desc: |
          Surface any claim past its TTL on its own line(s):
            Stale: <id> "<title>" — claimed by <actor>, expired <dur> ago
          Forest scope: every stale claim in the db. Subtree scope:
          stale claims under the argument task. Multiple stale claims
          render as multiple lines; omit the block entirely when none
          are stale. Recovery signal for "an agent crashed, this work
          needs picking up."
        labels: [cli, status, claim]
        blockedBy: [u4, u5]

      - title: summary alias with stderr deprecation notice
        ref: u8
        desc: |
          `job summary [id]` continues to work but emits a one-line
          stderr notice on every call:
            summary is deprecated; use 'status' instead
          Matches the existing deprecation pattern for `ls` -> `list`,
          `show` -> `info`, and `block by` / `unblock from`. Zero
          behavior change — the command routes to the new `status`
          path and produces the same stdout output.
        labels: [cli, summary, docs]
        blockedBy: [u4, u5]
```
