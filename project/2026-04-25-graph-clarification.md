# Mini-graph clarification: the subway system model

## Metaphor

The mini-graph is a **subway system map**. It is a wayfinding tool, not a Gantt
chart and not a tree visualization. The question it answers is the same one a
rider asks at a station:

> "What stop am I on, and how does that relate to the other stops I need to
> get to?"

Every design decision below serves that question. When a rule starts to feel
abstract, return to the metaphor: lines, stops, transfers, construction
closures.

## Why the previous model was confusing

The old graph compressed the tree topologically and stacked any open sibling
of the focal that didn't fit on the spine. Two problems:

1. **Branching broke the timeline metaphor.** The spine read as time
   (left = past, right = future), but stacked siblings broke that — they
   weren't earlier or later, just elsewhere in the tree.
2. **One node could implicitly mean multiple things.** A "stacked" sibling
   wasn't on the timeline; it wasn't really off it either. Readers had to
   reason about which lane a node belonged to, and the dashed blocker arcs
   compounded the ambiguity.

The fix is to commit to the subway-system metaphor end-to-end.

## The model

- **Lines (rows).** Each line corresponds to a parent whose subtree contains
  active or imminent work. A line shows that parent and its children, in tree
  order, as stops along the line.
- **Stops (nodes).** Each child renders once on its parent's line. No node
  ever appears twice in the graph.
- **Transfers (the LCA fork).** When two or more lines exist, they branch
  from a single fork node — the lowest common ancestor of all rendered
  parents. The fork is a transfer station: the place where you change lines.
- **Highlights.** Active claims render as `[X]`. Done stops within the
  visible window render as `X✓`. Plain stops are available, unclaimed work.
- **Construction closures.** When a line cannot yet be entered (because a
  prior sibling phase is incomplete), the closure marker `⊘` annotates the
  *edge* connecting the fork to that line — not the line's stops. The line is
  fine; the connection is closed.

## Rules

### Lines

A line exists for every parent whose subtree contains either:

- an active claim, or
- a stop reached by lookahead from any active claim (see below).

The line's stops are the parent's children. The parent itself renders as the
left anchor of the line.

### Lookahead

From each active claim, walk forward `+L` leaves in tree-traversal order
(next sibling, ascending into the next phase if the current phase is
exhausted). Default `L = 2`. Any parent encountered along that walk gets a
line, even if nothing in it is claimed — that's how the reader sees what's
about to light up.

### Elision

Each line shows a window of `±N` siblings around any visible node (focal,
done within the window, or lookahead-touched). Default `N = 2`. Stops outside
the window render as `…`.

When a line has **multiple focals** that fall outside each other's windows,
render the union of their windows separated by `…`. Two focals far apart on
the same line read as two lit windows on one track, which is honest.

The line's parent (left anchor) always renders; elision happens between the
anchor and the visible window when there are skipped stops.

### Done siblings

Done siblings render only when they fall inside a visible window:

- Recent done siblings immediately before a focal (left context).
- Done siblings *between* two focals on the same line (so the line stays
  visually continuous).

Older or unrelated done work does not render. When a parent's entire subtree
is done, that line drops out — done phases aren't part of "what's happening
now."

### Blocks

The closure marker `⊘` lives on the connector edge from the fork to the
blocked line's parent, not on the parent node itself.

```
   A
    ⊘
     G → H → I
```

This communicates "this line is available but currently closed for
construction" rather than "this stop is broken." For *sequential phase*
blocks (G blocked because B isn't done yet), `⊘` alone is sufficient. For
*explicit `blocks` edges* between unrelated jobs, a distinct glyph or color
on the edge can disambiguate (open question — see below).

### Sibling order

Stops within a line follow **tree order**, not claim order. This keeps the
layout stable as claims churn — a stop doesn't shuffle horizontally just
because someone claimed or released it.

### Fork rendering

When two or more lines exist, the LCA fork renders as a single node from
which every line branches. One line's left anchor renders inline with the
fork (`A → G → ...`) for visual flow; the others connect via `/` and `\`
branches. The inline choice is cosmetic — typically the middle line for
three lines, the top for two.

When only one line exists, the fork is omitted entirely. The LCA would be
chrome.

## Scenarios

Reference tree:

```
A: Implement dashboard
  B: Phase 1 — Front-end
    C: Folder structure
    D: CSS variables
    E: CSS files
    F: HTML files
  G: Phase 2 — Javascript     (blocked by B)
    H: Install test library
    I: Wire JS to front-end
  J: Phase 3 — SQL Database
    K: Install SQLite
    L: Create model files
```

### Scenario 1 — D claimed (C done)

Single focal in B's subtree. Lookahead stays within B. One line, no fork.

```
B → C✓ → [D] → E → F
```

### Scenario 2 — D and E claimed (siblings)

Two focals on B's line, rendered as adjacent lit stops. Lookahead from E
reaches G (next phase), which is blocked by B — G's line appears with `⊘` on
the ingress edge.

```
     B → C✓ → [D] [E] → F
    /
   A
    ⊘
     G → H → I
```

### Scenario 3 — D claimed, F claimed (E done between)

Both focals in B. E renders inline as done because it sits between two
visible focals. G appears as a closed line.

```
     B → C✓ → [D] → E✓ → [F]
    /
   A
    ⊘
     G → H → I
```

### Scenario 4 — D claimed, H claimed (G unblocked)

Two focals across two phases. Lookahead from H reaches K (in J), so J also
appears as a peek line. Three lines total, fork at A.

```
       B → C✓ → [D] → E → F
      /
     A → G → [H] → I
      \
       J → K → L
```

### Scenario 5 — D claimed, K claimed

LCA is A; G is untouched (no claim, no lookahead reaches it). Two lines.

```
     B → C✓ → [D] → E → F
    /
   A
    \
     J → [K] → L
```

### Scenario 6 — H claimed, K claimed (B fully done)

B's subtree is complete, so its line drops out. Two lines, fork at A.

```
   A → G → [H] → I
    \
     J → [K] → L
```

## Edge cases worth pinning

- **Three or more active phases.** Rules generalize: each row is independent,
  fork branches from A to each. Layout budget caps practical row count
  (likely 4–5 in the dashboard slot before truncation).
- **Deep LCA path.** If the LCA is several levels above the row-parents
  (e.g. `A → M → B` and `A → M → G`), render the full LCA-to-fork path
  inline, with the fork at the actual divergence point. Don't collapse
  intermediate ancestors silently.
- **Same agent, multiple claims.** Treated identically to multi-agent claims
  for layout purposes. The graph is about work, not workers.
- **Mid-row deep focal.** If a focal is several levels below its line's
  parent (e.g. claim is on a grandchild), the line still anchors at the
  nearest ancestor with siblings worth showing. Strictly: the line's parent
  is the deepest ancestor of the focal whose other children are relevant
  context. (Refinement may be needed once we hit real cases.)
- **Sibling claims in disjoint subtrees that bypass A's children.** Same
  rules apply; the fork is at the LCA, even if that's deeper than A.

## Open questions

- **Default values for `N` (window size) and `L` (lookahead depth).** Current
  proposal: `N = 2`, `L = 2`. Worth tuning against real screenshots.
- **Sequence-block vs explicit-block glyph.** `⊘` covers sequential phase
  blocks. Explicit `blocks` edges between unrelated jobs may want a distinct
  marker (different glyph, dashed style, or color) so the reader can tell
  "waiting for prior phase" from "waiting for specific dependency."
- **Group/phase labels.** Current scenarios use the parent node itself as the
  line anchor. Whether to also render a textual phase label (e.g. "Phase 2 —
  Javascript") above each line is a presentation question — useful in wide
  layouts, possibly noisy in narrow ones.
- **Done-phase indicators.** Scenario 6 silently drops B's line because B is
  done. Should there be a tiny "Phase 1 ✓" marker at the fork as historical
  context, or does that violate the "what's happening now" principle?
- **Truncation rules.** When more lines exist than fit, which lines are
  shown? Likely: lines with active claims first, peek-ahead lines second,
  with a "+N more" indicator.

## Migration notes

The current implementation (`internal/web/handlers/graph.go`,
`collectStackedSiblings` and friends) is doing topological compression and
sibling stacking. Model D is a substantial rewrite, not a tweak:

- Stacked siblings go away as a concept.
- The single-spine layout becomes multi-line layout with an LCA fork.
- The blocker-arc logic is replaced by edge-level closure markers.
- The mini-graph test (`TestMiniGraph_VerticalSiblingStacking`) will need to
  be replaced with tests covering the line/fork/closure model and each of
  the six scenarios above.

We are pre-launch (BUILD mode), so no backward compatibility is needed.

## Implementation plan

```yaml
tasks:
  - title: Mini-graph subway-system rewrite
    desc: |
      Implement the multi-line subway-system mini-graph per the model
      described above. Replaces the current single-spine layout with
      stacked siblings.
    labels: [graph, web]
    children:
      - title: Phase 1 — Layout algorithm
        ref: layout
        children:
          - title: Define line / fork / window data structures
            desc: |
              New types: a Line (parent anchor + ordered stops with
              elision markers), a Fork (LCA + branches), and a Layout
              that owns one or more Lines plus an optional Fork.
          - title: Red tests for line collection (Scenarios 1–6)
            desc: |
              Given a set of focals, assert which parents become lines.
              Covers single-line (no fork), peek-ahead from lookahead,
              dropped done-only subtree, and disjoint-subtree forks.
          - title: Implement line collection with +L lookahead
            desc: |
              Walk each focal forward L=2 leaves in tree-traversal
              order. Any parent whose subtree contains a focal or a
              touched stop becomes a line.
          - title: Red tests for LCA fork computation
            desc: |
              Two-line, three-line, and deep-LCA-path (A → M → B vs
              A → M → G) cases.
          - title: Implement LCA fork computation
          - title: Red tests for ±N windowing with multi-focal union
            desc: |
              Default N=2 around any visible node. Multi-focal-union
              case: two focals far apart on one line render as two
              visible windows separated by `…`.
          - title: Implement per-line windowing with elision markers
      - title: Phase 2 — Rendering
        ref: render
        blockedBy: [layout]
        children:
          - title: Snapshot tests for Scenarios 1–6
            desc: |
              Render-level tests asserting the SVG output matches the
              documented diagrams for each scenario.
          - title: Implement multi-line SVG layout
            desc: |
              Single-line case: no fork. 2+ lines: render the fork once
              with one line's anchor inline (middle for 3, top for 2)
              and others branching via /  \.
          - title: Red tests for edge-level closure-marker placement
            desc: |
              `⊘` annotates the ingress edge to a blocked line, not
              the line's anchor node. Covers Scenarios 2 and 3.
          - title: Implement closure-marker rendering
          - title: CSS for fork, connectors, closures, lit stops, elision
      - title: Phase 3 — Integration & cleanup
        blockedBy: [render]
        children:
          - title: Retire stacked-sibling code path
            desc: |
              Remove collectStackedSiblings and supporting logic in
              internal/web/handlers/graph.go.
          - title: Retire TestMiniGraph_VerticalSiblingStacking
          - title: Add edge-case tests
            desc: |
              3+ active phases, deep LCA path, mid-row deep focal,
              same-agent multiple claims.
          - title: Update docs in docs/content/docs/
            desc: |
              Replace the current mini-graph description with the
              subway-system model.
      - title: Phase 4 — Open-question decisions
        desc: |
          Decision tasks, not coding. Resolve each as the
          implementation lands and we can eyeball it on real data.
        blockedBy: [render]
        children:
          - title: Tune defaults for N (window) and L (lookahead)
          - title: Decide on glyph for explicit `blocks` edges
            desc: |
              `⊘` covers sequential phase blocks. Pick a distinct
              visual (different glyph, dashed style, or color) for
              explicit blocks between unrelated jobs.
          - title: Decide whether to render phase labels above lines
          - title: Decide on done-phase indicator at the fork
            desc: |
              Show "Phase 1 ✓" at the fork when a phase is fully done,
              or drop it entirely (current spec).
          - title: Decide on truncation rules for >N lines
            desc: |
              Active-claim lines first, peek-ahead second, with a
              "+N more" indicator.
```

