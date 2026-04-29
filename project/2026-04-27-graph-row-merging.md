# Mini-graph row-merging — design note

Date: 2026-04-27
Status: design — not yet implemented

This note captures a redesign of the home dashboard's mini-graph
("subway" graph) row-and-window logic. It supersedes the rules
landed in `Tx8rV` (lookahead window dedup) and `z6D7h` (lookahead
parent-boundary rule); both of those were partial steps toward what
this note describes. It accompanies and partially supersedes
`project/2026-04-25-graph-clarification.md`.

The model has two render modes — **single-focal preorder window**
and **multi-focal tree map** — and a small set of invariants. The
visual primitives (node discs, flow edges, in-gap ellipsis dots)
stay the same; the organizing principle changes between modes.

## Two modes

The mini-graph answers two distinct questions, and the rendering
needs to match the question being asked:

- **One focal → "where are we?"** A temporal/narrative view.
  Linear preorder walk, depth-fluid, pulls in whatever history fits
  in `-N` steps and whatever frontier fits in `+N` steps. The row
  reads as a left-to-right work order.

- **Two or more focals → "how are these parallel pieces of work
  related?"** A structural/tree view. Branching at every divergence
  (with one carve-out for same-parent siblings). The row layout
  reads as a tree map, with the depth ladder visible on the left
  edge.

The dispatch razor is **"can this root render as a single row?"**
If the focal-path subgraph collapses to a single linear chain (one
focal, or multiple focals all on the same branch with no
divergence), the root renders in single-focal mode. Otherwise
multi-focal. The common case — `len(focals) == 1` — falls out of
this rule, but the rule is structural, not a count:

| Focal-path subgraph shape | Mode                         |
|---------------------------|------------------------------|
| Single chain              | single-focal preorder window |
| Branching                 | multi-focal tree map         |

Cross-project claims (focals in disjoint trees) produce one
independent rendering per project root, stacked vertically. **Mode
dispatch is per-cluster:** each root chooses its own mode based on
its own focal-path subgraph. Project A with one focal renders
single-focal alongside project B with two divergent focals
rendering multi-focal.

## Invariants that hold in both modes

These are non-negotiable; they make the rendering honest and the
layout tractable.

### 1. Focals are leaves only

Parent claims are allowed at the data layer (legacy; the
leaf-frontier rule normally prevents them) but `pickFocals` filters
them out. A focal is always a leaf with no open children. This
guarantees that focals never split into parents-with-subtrees during
rendering.

### 2. Each tree node renders at most once

If the slice would otherwise render a node twice (e.g., as a stop on
one row and the leftmost of another), the layout suppresses the
duplicate. Every node has exactly one `(row, col)` position. This
forecloses the entire class of "two anchors collide" layout bugs.

### 3. Branch curves originate at a leftmost-of-row

Every fork edge starts at the leftmost node of some row above and
ends at the leftmost node of the diverging row. The depth-alignment
rule (below) plus invariant 2 together guarantee this without
needing recursive promotion of mid-row stops.

### 4. Ellipsis rendering

The row's leftmost is **always rendered** — it's the structural
anchor (col 0 in single-focal, the depth-aligned divergence point
in multi-focal) and the target of any incoming branch curve.
Elision never sits *left of* leftmost; it sits *between* leftmost
and the row's content when the `-N` window doesn't span the gap.

- **Backward elision** (gap between leftmost and the `-N`-th
  content stop): rendered as a *broken line* — short stub from
  leftmost, three small SVG circles in negative space, short stub
  into the first visible content node. No continuous line passing
  through the dots.
- **Forward elision** (the row's preorder walk continues past `+N`
  but we're cutting it off): the row *terminates* at the ellipsis.
  Disc → segment → three dots, no trailing arrow stub.
- **Mid-line elision** (between two non-adjacent visible windows
  inside a single row): same broken-line treatment as backward.
- No `(+N)` count anywhere. Plain ellipsis is the only marker.
  Counts proved fragile — different reasonable counts (siblings of
  focal? all elided preorder steps? leaves only?) all coexist
  without one being clearly right, and the number invites the reader
  to trust it.

### 5. SVG centers the rendered extent

The layout positions things at `col = depth` (multi-focal) or
`col = 0` (single-focal), then computes the rendered bounding box
and centers it horizontally inside the SVG. The two modes can
produce different left-anchored extents; centering hides the
difference.

## Single-focal preorder window mode

There is exactly one focal `F` (either a claim or `globalNext` when
no claims exist). The slice is the linear preorder walk of the
project tree containing `F`.

### Window

- Backward window (`-N` preorder steps before `F`): renders as a
  *path* showing how we got to the focal. Mixes ancestors crossed
  on the way down with prior-sibling leaves and great-uncles
  encountered in the walk. Whatever preorder finds, in tree order.
- Forward window (`+N` preorder steps after `F`): renders as a
  *frontier* showing what's next. Mixes descendants of the focal
  (if it had any — but focals are leaves, so this case doesn't
  arise here), siblings, parent boundaries crossed on the way out,
  and so on.

The asymmetry is meaningful: backward is *history* (how the work
got to where it is), forward is *frontier* (what work is upcoming).
Both are preorder-faithful.

### Layout

- Leftmost node of the row sits at `col = 0`. Subsequent stops
  advance by one column each, regardless of depth changes within
  the walk.
- Backward elision (broken-line) appears between the row's leftmost
  and the actual `-N`-th step when the walk doesn't naturally
  terminate at the leftmost — i.e., when there is more history that
  was not surfaced. If the walk naturally starts at the project
  root and `-N` covers all of it, no leading elision.
- Forward elision (terminating dots) appears at the row's right
  edge when the preorder walk continues past `+N`.

### Example: `?at=1288`

Focal: `k9fFC` ("Dynamic favicon"). Project root: `bYr6R`
("Web dashboard v1"). Preorder walk from `bYr6R` includes Phase 1
through Phase 9's children. With ±2 around `k9fFC`:

```
… → 1SYqo ✓ → hNTiB ✓ → [k9fFC] → oDKYC ✓ → tpC4u ✓ → …
```

Backward broken-line elision swallows everything before `1SYqo`
(many done phases). Forward terminating elision covers the rest
of the tree (Phase 10's children, etc.). One row.

## Multi-focal tree-map mode

There are two or more focals. The render becomes structural: a
tree map of the focal-path subgraph.

### Focal-path subgraph

Take all focals. Mark every node on the path from the LCA of all
focals down to each focal — that's the focal-path subgraph. The
subgraph is a tree rooted at the LCA, with focals at its leaves.

### Fork rule

A node in the focal-path subgraph with **two or more in-subgraph
children** is a fork point. Each fork point splits its in-subgraph
children into independent rows. Recurse: a child row may itself
contain fork points and split further.

A row is a maximal linear chain through the focal-path subgraph
that contains no fork points.

### Carve-out: same-parent sibling focals

Two focals that are direct siblings under the same parent share a
row, even though by the strict fork rule the parent would be a
fork point. The row visits both focals as adjacent stops. This
matches the existing subway intuition that "a parent's children
are a line of stops on that parent's track."

If three or more sibling focals share a parent, the carve-out
extends: all of them ride on the parent's row as adjacent stops.

**Non-focal siblings between focal stops:** when two focal siblings
have non-focal siblings between them in tree order, **≤2** of those
non-focal siblings render inline as adjacent stops on the row;
**≥3** collapse to mid-line broken-line elision between the focal
stops. So focals A · (one or two non-focals) · C renders inline;
A · (three+ non-focals) · C renders as A · ⋯ · C.

If a same-parent-siblings group's parent ALSO has a non-sibling
focal-bearing child (a deeper subtree with a focal of its own),
the parent is still a fork point: the sibling row is one row, the
deeper subtree is another row, both branching from the parent.

### Depth-aligned leftmost

Each row's leftmost node sits at `col = its depth` in the project
tree. This makes the left edge of the multi-row graph a literal
depth ladder: cols 0, 1, 2, … hold ancestors at successive depths,
and the rendered tree visually mirrors the actual tree on the left.

The top row's leftmost is the LCA of all focals in the cluster
(typically the project root for a single-cluster Subway).

### Branch curves

For each row except the topmost, draw a branch curve from
`row.leftmost.parent`'s rendered position to `row.leftmost`. Since
both endpoints are leftmost-of-row nodes (invariant 3), the curve
is always a clean diagonal from `(row=N, col=K)` to
`(row=N+M, col=K+1)`.

### Window inside a row

Each row has its own focal(s) and its own ±N window, computed
within the row's branch only. The window machinery is identical to
single-focal mode: walk `-N` content steps back from the focal and
`+N` forward, all within the row's branch. The row's structural
leftmost is always rendered as the curve target; if `-N` doesn't
reach the leftmost's first child, broken-line elision sits *between*
leftmost and the first visible content stop. Forward terminating
elision sits at the right edge when the branch continues past `+N`.

**Example: deep focal on one branch, shallow focal on a sibling
branch.** Two sibling branches under a common parent. Branch 1's
focal is step 80 of 84; Branch 2's focal is step 3 of 4.

```
Col 0    Col 1            Col 2    Col 3    Col 4      Col 5    Col 6
Parent   Branch 1 -•••-   Step 78  Step 79  [Step 80]  Step 81  Step 82 -•••
         Branch 2         Step 1   Step 2   [Step 3]   Step 4
```

Branch 1's row has leading broken-line elision (between `Branch 1`
and `Step 78`) because `-2` from `[Step 80]` is steps 78–79, far
short of the structural leftmost. Branch 2's `-2` reaches all the
way back to `Step 1`, no leading elision needed. Branch 1 also has
trailing terminating elision because the branch continues past
`Step 82`. The branch curve from `Parent` lands on each row's
leftmost (`Branch 1` and `Branch 2`), both rendered.

### Example: parallel claims at different depths

Focals: `[Create HTML template]`, `[Write HTML tests]`,
`[Research test harness]`. Tree:

```
Front-end
  Prototype
    HTML
      [Create HTML template]
      HTML tests
        [Write HTML tests]
        Install test harness
          [Research test harness]
```

Focal-path subgraph: Prototype, HTML, Create HTML template, HTML
tests, Write HTML tests, Install test harness, Research test
harness.

Fork points:
- HTML has two in-subgraph children (Create HTML template, HTML
  tests) → fork.
- HTML tests has two in-subgraph children (Write HTML tests, Install
  test harness) → fork.

Rows:

```
col 0      col 1  col 2                    col 3
Prototype  HTML   [Create HTML template]
                  HTML tests               [Write HTML tests]
                                           Install test harness   [Research test harness]
```

(`Install test harness` actually sits at col 3 since its depth is 3,
and `[Research test harness]` at col 4 — the example shifts slightly
in real pixel layout but the logic stands.)

### Example: same-parent-siblings carve-out

Focals: `[Create HTML template]` and `[Write HTML tests]` where both
are direct children of HTML. Subgraph: HTML, Create HTML template,
Write HTML tests.

Strict fork rule says HTML is a fork point. Carve-out says: same
parent → one row.

```
col 0  col 1  col 2
HTML   [Create HTML template]   [Write HTML tests]
```

One row, two focals as adjacent stops. (HTML's other children, if
any, are absorbed via the row's window or elided.)

## What this changes from the current implementation

- `pickFocals` gains a leaf-only filter: skip claimed parents.
- `collectLines` becomes `collectClusters` (or similar): groups
  focals by project root, and within a root produces a single
  cluster — not a per-parent draft. `lineSeed` shifts from "parent
  + focals + lookaheads" to "cluster + focal-path subgraph + window
  parameters."
- `applyWindow` becomes mode-aware. Single-focal mode walks
  preorder and emits a flat `[]LineItem`. Multi-focal mode walks
  the focal-path subgraph, identifies fork points, applies the
  same-parent-siblings carve-out, and emits a `[]Row` where each
  row is a flat `[]LineItem` with a `parentShortID` for its branch.
- `LineItemMore` (terminal `(+N)` pill) is removed entirely;
  trailing elisions render as terminating ellipsis dots.
- The fork machinery (`Fork`, `SubwayForkView`) generalizes: each
  non-top row has a `ParentShortID`, and the layout draws the curve
  from the parent's rendered position to the row's leftmost.
- The current single-LCA `AncestorChain` gets retired in favor of
  per-row parent edges; the depth-aligned leftmost rule replaces
  the "anchorCol = maxChain" allocation.
- Layout pixel positioning shifts from "everyone at anchorCol" to
  "leftmost at depth, then walk right." A final centering pass
  shifts the whole rendering horizontally to balance the SVG.
- Edge geometry: the same-column vertical-line band-aid in
  `buildSubwayEdgeView` becomes unreachable and can be removed
  along with its regression test.

## Implementation order

A reasonable sequence (each step gated by red tests):

1. **Single-focal preorder window mode.** Replace the parent-rooted
   line with a preorder slice over the project tree containing the
   focal. ±N counts preorder steps. Backward broken-line elision,
   forward terminating elision. Drop `LineItemMore`. This case
   covers the dashboard's most common rendering and should land
   first.

2. **Multi-focal tree-map mode.** Build the focal-path subgraph,
   identify fork points, apply the same-parent-siblings carve-out,
   emit per-row `[]LineItem`. Update `Fork`/`SubwayForkView` to
   per-row parent edges.

3. **Layout pivot.** Depth-aligned leftmost; per-row column
   advancement; final centering pass. Remove `AncestorChain`
   special-casing in column allocation.

4. **Cleanup.** Remove the same-column vertical-line branch in
   `buildSubwayEdgeView` and its regression test. Delete the
   `(+N)` pill rendering from the template and CSS.

5. **`pickFocals` leaf-only filter.** Add the
   `if len(t.children) > 0 { continue }` guard so parent-claims are
   never focals. Update any tests that relied on parent-claim focals
   to use leaf claims (most already do).

## Open questions for the implementation phase

These are intentionally not specified yet — flagged here so they
get discussed when the relevant step lands.

- **`N` value.** Currently 2 (production). Single-focal mode might
  benefit from a larger window since the row is doing more narrative
  work. Worth measuring empirically against real dashboard frames.

- **Mode boundary.** What happens at the moment a second focal
  appears or disappears? The render switches modes — is the
  transition disruptive enough to want a mode-stable interpolation,
  or is the disruption itself useful signal ("you just gained a
  parallel claim")?

- **Cross-project rendering.** Two project roots each with their
  own rendering, stacked vertically. Mode dispatch is per-cluster.
  Worth confirming the inter-project gap and whether the centering
  pass operates per cluster or globally.

- **Empty windows.** If `+N` extends past the focal's natural
  preorder end (the focal is the last leaf in its row's branch),
  the row terminates at the focal — no forward elision. Worth
  spelling out so the layout doesn't emit a stray terminating
  ellipsis. Likewise, if a project root has zero open leaves
  (everything done), the graph renders a "No open tasks"
  placeholder rather than an empty SVG.

## References

- `project/2026-04-25-graph-clarification.md` — original subway-map
  design. The high-level metaphor (lines, transfer stations,
  closure markers) survives; the row-and-window logic in this note
  supersedes that doc's per-parent-line model.
- `Tx8rV` (commit on main) — first attempt at lookahead dedup.
  Index-level dedup; superseded by the parent-boundary rule below.
- `z6D7h` (commit on main) — parent-boundary rule for lookahead
  absorption. Closer to the right model but still per-parent;
  superseded by the cluster + tree-map model in this note.
