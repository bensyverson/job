# Graph row-merging redesign — Jobs tree

Children of `wQtfX` (Implement graph row-merging redesign per
project/2026-04-27-graph-row-merging.md). Five steps from the
design doc, each gated by red tests, pinned to current file:line
pointers from the 2026-04-27 code survey.

## File:line landmarks (current state, pre-redesign)

- `pickFocals` — `internal/web/signals/graph.go:133–157` (no leaf filter)
- `collectLines` — `internal/web/signals/subway.go:444–564`
- `lineSeed` — `internal/web/signals/subway.go:429–433`
- `applyWindow` — `internal/web/signals/subway.go:597–672`
- `LineItem` + `LineItemMore` — `internal/web/signals/subway.go:299–330`
- `Fork` (single-element `AncestorChain`) — `internal/web/signals/subway.go:342–345`
- `SubwayForkView` — `internal/web/render/subway_layout.go:50–53`
- `LayoutSubway` (anchorCol = maxChain) — `internal/web/render/subway_layout.go:150–351`
- `buildSubwayEdgeView` (same-column band-aid) — `internal/web/render/subway_layout.go:354–408`
- Same-column regression test — `internal/web/render/subway_layout_test.go:933–967`
- Template `(+N)` pill — `internal/web/templates/html/pages/home.html.tmpl:288–292`
- `.c-subway-more` CSS — `internal/web/assets/css/components.css:2630–2646`

## Import plan

```yaml
tasks:
  - title: "S1: Single-focal preorder window mode"
    ref: s1
    labels: [graph, web]
    desc: |
      Replace the parent-rooted line with a preorder walk over the
      project tree containing the single focal. ±N counts preorder
      steps. Leftmost is always rendered; backward broken-line
      elision sits between leftmost and the -N-th content stop;
      forward terminating elision when the walk continues past +N.
      Drops LineItemMore from the rendering pipeline (template/CSS
      removal happens in S4, but the data path stops emitting it
      here). Mode dispatch via the "single chain vs branching"
      razor on the focal-path subgraph.
    children:
      - title: "S1a red: single-focal preorder window tests"
        ref: s1a
        desc: |
          Failing tests in subway_test.go for the new model:
          - one focal, walk -2/+2 preorder steps; leftmost rendered;
            no leading elision when walk reaches project root
          - one focal mid-tree, -2 doesn't reach root → broken-line
            elision between leftmost and -2-th content stop
          - one focal near tree end, +2 walks past last leaf → row
            terminates at focal, no trailing elision
          - one focal, +2 cuts row mid-walk → terminating ellipsis
            at right edge, no (+N) count anywhere
          - parent-boundary crossing in window: parent node renders
            as its own preorder stop (e.g., Phase 7 leaf → Phase 8 →
            Phase 8 leaf → [focal])
          - reproduces ?at=1288 (focal k9fFC, ±2): row reads
            … → 1SYqo ✓ → hNTiB ✓ → [k9fFC] → oDKYC ✓ → tpC4u ✓ → …
          Verify all fail against current code before implementing.
      - title: "S1b green: implement preorder window for single-focal"
        ref: s1b
        blockedBy: [s1a]
        desc: |
          Add a single-focal code path in subway.go that walks the
          project-tree preorder around the focal and emits a flat
          []LineItem (Stop + Elision only, no More). Dispatch from
          the entry point on the focal-path subgraph: if it
          collapses to a single chain, use the new path; otherwise
          fall through to the existing multi-focal machinery
          (replaced wholesale in S2). The "single chain" predicate
          covers len(focals) == 1 and all-focals-on-one-branch.
      - title: "S1c green: stop emitting LineItemMore"
        ref: s1c
        blockedBy: [s1b]
        desc: |
          Forward elision becomes a terminating ellipsis instead of
          a (+N) pill. Stop populating LineItemMore in applyWindow
          (subway.go:597–672) for both code paths. Template/CSS
          removal happens in S4; this step just stops feeding it.
          Existing tests asserting (+N) counts get updated to
          assert terminating ellipsis — explain each test edit in
          the commit (per CLAUDE.md TDD rule).

  - title: "S2: Multi-focal tree-map mode + per-row parent edges"
    ref: s2
    blockedBy: [s1]
    labels: [graph, web]
    desc: |
      Build the focal-path subgraph (LCA-down to each focal),
      identify fork points (in-subgraph nodes with ≥2 in-subgraph
      children), apply same-parent-siblings carve-out (≤2 non-focal
      siblings inline / ≥3 collapsed), and emit []Row where each
      row carries its own []LineItem and a parentShortID for its
      branch curve. Each row's leftmost is always rendered. Per-row
      ±N window with broken-line backward elision between leftmost
      and -N-th content stop; terminating forward elision.
    children:
      - title: "S2a red: focal-path subgraph + fork detection tests"
        ref: s2a
        desc: |
          Failing tests for:
          - LCA computation across focals in disjoint depths
          - fork-point identification (≥2 in-subgraph children)
          - linear chain through subgraph splits at every fork
          - non-focal-bearing branches excluded from subgraph
      - title: "S2b red: same-parent-siblings carve-out tests"
        ref: s2b
        desc: |
          Failing tests:
          - 2 focal siblings under same parent → one row, both as
            adjacent stops (parent NOT a fork point)
          - 3+ focal siblings under same parent → one row, all
            adjacent
          - 2 focal siblings + 1 non-focal sibling between them →
            inline (≤2 rule)
          - 2 focal siblings + 2 non-focal siblings between → inline
          - 2 focal siblings + 3 non-focal siblings between → mid-line
            broken-line elision between focal stops
          - same-parent-siblings group + non-sibling focal-bearing
            child of parent → parent IS a fork point; sibling row +
            deeper subtree row both branch from parent
      - title: "S2c red: per-row window + leftmost rendering tests"
        ref: s2c
        desc: |
          Failing tests:
          - row's structural leftmost (depth-aligned divergence
            point) always rendered
          - -N from row's focal doesn't reach leftmost's first
            child → broken-line elision between leftmost and the
            -N-th stop
          - +N continues past row's branch → terminating ellipsis
          - branch curve from parent's rendered position lands on
            row's leftmost
          - reproduces "step 80/84 vs 3/4 sibling branches" example
            from design doc
      - title: "S2d green: implement focal-path subgraph + rows"
        ref: s2d
        blockedBy: [s2a, s2b, s2c]
        desc: |
          Replace collectLines (subway.go:444–564) with a
          cluster/subgraph builder. Replace lineSeed
          (subway.go:429–433) with a Row type carrying focal(s),
          parentShortID, and the per-row window inputs. Replace
          applyWindow's per-parent logic with per-row windowing
          using the same window machinery as the single-focal path
          from S1.
      - title: "S2e green: per-row parent edges via Fork/SubwayForkView"
        ref: s2e
        blockedBy: [s2d]
        desc: |
          Generalize Fork (subway.go:342–345) so each non-top row
          has a ParentShortID, and SubwayForkView
          (subway_layout.go:50–53) draws the curve from the
          parent's rendered position to the row's leftmost. The
          single-element AncestorChain field is retired here (or in
          S3, whichever lands first); coordinate with S3.

  - title: "S3: Layout pivot — depth-aligned columns + centering"
    ref: s3
    blockedBy: [s2]
    labels: [graph, web]
    desc: |
      Replace the all-anchors-at-maxChain rule
      (subway_layout.go:161–187) with depth-aligned leftmost: each
      row's leftmost sits at col = depth(leftmost). Subsequent
      stops on the row advance by one column each, regardless of
      depth changes within the walk (single-focal mode). Final
      pass computes the rendered bounding box and centers it
      horizontally inside the SVG. Retires AncestorChain.
    children:
      - title: "S3a red: depth-aligned leftmost tests"
        ref: s3a
        desc: |
          Failing tests in subway_layout_test.go:
          - top row's leftmost at col = depth(LCA)
          - each non-top row's leftmost at col = depth(leftmost)
          - single-focal row's leftmost at col 0
          - centering pass: rendered extent horizontally centered
            in SVG independent of depth offset
      - title: "S3b green: implement depth-aligned columns"
        ref: s3b
        blockedBy: [s3a]
        desc: |
          Rewrite the column-allocation block in LayoutSubway
          (subway_layout.go:150–351). Drop maxChain / anchorCol
          computation. Each row places leftmost at col = depth, then
          walks right one col per stop. Multi-focal rows with
          deeper structural leftmosts get correctly indented; the
          left edge becomes a literal depth ladder.
      - title: "S3c green: centering pass"
        ref: s3c
        blockedBy: [s3b]
        desc: |
          Add a final pass that computes the rendered bounding box
          and shifts the whole rendering horizontally to center it
          in the SVG viewport. Per-cluster centering for
          cross-project (multi-root) renderings — confirm with the
          open-question on cross-project gap before merging.
      - title: "S3d green: retire AncestorChain"
        ref: s3d
        blockedBy: [s3b]
        desc: |
          Remove Fork.AncestorChain (subway.go:342–345) and
          SubwayForkView.AncestorShortIDs (subway_layout.go:50–53)
          if not already removed in S2e. The depth-aligned-leftmost
          rule plus per-row parent edges replaces it.

  - title: "S4: Cleanup — same-column band-aid + (+N) pill remnants"
    ref: s4
    blockedBy: [s3]
    labels: [graph, web]
    desc: |
      With invariant 2 (each node renders at most once) enforced
      end-to-end, the same-column vertical-line case in
      buildSubwayEdgeView becomes unreachable and can be removed
      along with its regression test. The (+N) pill is gone from
      the data path in S1; remove its template and CSS too.
    children:
      - title: "S4a: remove same-column vertical-line band-aid"
        ref: s4a
        desc: |
          Delete the fromCX == toCX && fromCY != toCY branch in
          buildSubwayEdgeView (subway_layout.go:362–372). Delete
          its regression test
          (subway_layout_test.go:933–967,
          TestLayoutSubway_SameColumnDifferentRow_RoutedVertically).
          Verify by running the full subway test suite; if any
          test now hits this code path, that's a redesign bug, not
          a cleanup blocker.
      - title: "S4b: remove (+N) pill from template + CSS"
        ref: s4b
        desc: |
          Delete the .Mores iteration block in
          home.html.tmpl:288–292 and the .c-subway-more class in
          components.css:2630–2646. Also delete the LineItemMore
          kind and the MoreCount field from the LineItem struct
          (subway.go:299–330) — no longer populated after S1c.

  - title: "S5: pickFocals leaf-only filter"
    ref: s5
    blockedBy: [s2]
    labels: [graph, web]
    desc: |
      Add the leaf-only filter to pickFocals
      (graph.go:133–157) so claimed parents are never returned as
      focals. The leaf-frontier rule normally prevents parent
      claims, so this is a safety net for legacy data.
    children:
      - title: "S5a red: parent-claim filtering tests"
        ref: s5a
        desc: |
          Failing tests:
          - claimed parent with open children → not a focal
          - claimed leaf → focal
          - claimed parent with no open children → not a focal
            (already a leaf in the open-frontier sense, but safety:
            keep the filter on len(t.children) > 0 only)
      - title: "S5b green: add leaf-only guard"
        ref: s5b
        blockedBy: [s5a]
        desc: |
          Add `if len(t.children) > 0 { continue }` to pickFocals.
          Update any tests that relied on parent-claim focals to
          use leaf claims (most already do per the survey).
```
