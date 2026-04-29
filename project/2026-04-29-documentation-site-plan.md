# Documentation site plan

**Date:** 2026-04-29
**Status:** Plan — pre-implementation
**Target URL:** https://bensyverson.com/documentation/Jobs/

---

## 1. Why

The README is now human-first: a short orientation that explains what Jobs is, what it isn't, and why anyone should care. It deliberately points the rest of the audience elsewhere — agents at `job --help`, depth-seekers at `docs/`. That `docs/` directory does not yet exist as a site. DOCS.md (the previous README) carries the agent-targeted reference today, but a single Markdown file isn't browsable, isn't linkable at section depth, and won't survive growth.

Jobs needs a real documentation site for three reasons:

1. **Agents are the primary reader.** They will arrive via `--help` and need a depth source for concepts (claims, leaves, criteria, identity), the plan grammar, and the JSON / HTTP surfaces. The site is the place that depth lives.
2. **Humans evaluating Jobs need somewhere to read.** The README sells; the site explains. A would-be user who is curious about acceptance criteria or multi-agent workflows shouldn't have to read source.
3. **`job schema` and the `/events` API are stable contracts that will drift if duplicated.** A built site with a generated schema partial keeps docs honest where prose alone would not.

The site is **not** a marketing surface, **not** versioned (BUILD mode), and **not** a place for design history (`project/` stays raw — it's part of the source for the curious, not the curated story).

---

## 2. Stack

Hextra on Hugo, identical to `organizize/docs/`:

- `docs/hugo.yaml` — site config (baseURL, menu, navbar, footer).
- `docs/go.mod` — pulls `github.com/imfing/hextra` as a Hugo module.
- `docs/content/docs/**` — Markdown content with frontmatter (`title`, `weight`).

No npm, no bundler, no JS runtime. Hugo + the Hextra module is the entire toolchain. CLAUDE.md already names `docs/content/docs/` as the canonical content path, so this is the previously-anticipated home.

`baseURL: https://bensyverson.com/documentation/Jobs/` — matches the existing personal-site documentation namespace.

---

## 3. Information architecture

Single living version. Sidebar order follows explicit `weight:` frontmatter. Concepts and Reference are terse — one tight page per topic, agent-optimized. Recipes is the only narrative section.

```text
docs/
  _index.md                       Landing — short pitch + hero screenshot + cards
  getting-started/
    _index.md                     Section landing
    install.md                    go install, PATH, AGENTS.md line
    initialize.md                 job init, identity at init time, .gitignore
    first-plan.md                 Author → import → claim → done walkthrough
  concepts/
    _index.md
    identity.md                   --as, default, strict mode, attribution
    leaves-and-claims.md          Leaf-frontier semantics, claim TTL, auto-extend, auto-release, auto-close
    criteria.md                   Acceptance criteria lifecycle, short_ids, override flags
    blockers.md                   block add/remove, cycle detection, auto-unblock on done
    labels.md                     Free-form labels, the `decision` convention
    events.md                     Event log model, append-only, what's recorded
  reference/
    _index.md                     Grouped command reference (mirrors `job --help` grouping)
    setup.md                      init, identity, schema
    planning.md                   add, import, edit, block, move, label, split
    execution.md                  claim, claim-next, release, note, done, reopen, cancel, heartbeat
    observation.md                ls, show, log, status, next, tail
    web.md                        serve (brief — see Web dashboard section)
  plan-grammar/
    _index.md                     Plan YAML overview, examples
    schema.md                     Auto-included `job schema` JSON via Hugo partial
  machine-interface/
    _index.md                     Why agents talk to Jobs as a service
    json-output.md                --format=json across read verbs, tail JSON-lines
    http-api.md                   /events SSE + JSON replay, query params, response shape
  web-dashboard.md                Brief — what serve does, who it's for, screenshot
  recipes/
    _index.md                     "Patterns that aren't in --help"
    great-plans.md                What makes a plan import cleanly + run smoothly
    criteria-as-tests.md          Criteria as first-draft unit tests
    multi-agent.md                Named identities, --as discipline, strict-mode bracketing
    recovery.md                   reopen, cancel, cancel --purge, when to use which
  contributing.md                 Package layout, migrations, test helpers, pre-commit hooks
```

Notes:

- **Concepts depth.** Each page ~½ screen. Goal: an agent can land on `concepts/criteria.md` from a `--help` reference and have the full mental model in one read.
- **Reference grouping.** Five pages by role, matching `job --help`'s grouping. Each page covers the verbs in that group at a level above `--help` — flags worth knowing, edge cases, idiomatic combinations — without repeating the help text verbatim. Showcases features that didn't make the base `--help`.
- **Plan grammar.** Hand-written prose page plus a `schema.md` whose body is rendered from a generated JSON partial (see §4).
- **Machine interface.** One section, two pages. Combines CLI JSON output and the HTTP `/events` API since both serve the same audience (programmatic consumers).
- **Web dashboard.** Single page. Brief description of `serve`, what each view does, a hero screenshot. Not a tutorial.
- **Recipes.** The only narrative section. Genuinely emergent patterns that can't be inferred from `--help`. Criteria-as-tests is a first-class entry — it's the most distinctive workflow Jobs enables.
- **`project/` not surfaced.** Repo-only. Treated as source-code-for-the-curious.

---

## 4. `job schema` generation

A `make docs-schema` target generates a JSON partial:

```
make docs-schema
  → job schema > docs/content/docs/plan-grammar/_schema.json
```

`docs/content/docs/plan-grammar/schema.md` includes the partial via Hugo's `{{ readFile }}` (or a custom shortcode) and renders it as a fenced code block with a "Generated from `job schema` — do not edit" banner.

`make docs` depends on `docs-schema`, so any local site build regenerates. No pre-commit hook (would slow every commit for a rarely-changing file). No CI (the project has none yet). The generated `_schema.json` is committed so GitHub renders correctly without a build step.

---

## 5. Hero screenshot

The site landing page (`docs/content/docs/_index.md`) carries one hero screenshot of the dashboard. Strictly for human readers evaluating the project. No screenshots elsewhere — the rest of the site is text-first.

The README's existing `[Screenshot]` placeholder also gets the same image.

---

## 6. DOCS.md

Deleted at the end of the migration. The site becomes the single source of truth. Any reference in the codebase to DOCS.md gets retargeted at the appropriate site page or removed.

---

## 7. Out of scope

- CI / GitHub Actions to deploy the site. Local `make docs` for now; deploy story is a follow-up once content lands.
- Versioning, version selector, `/v1/` URL prefix.
- Search beyond Hextra's default.
- Localization.
- Per-verb pages (the grouped reference is the deliberate choice).

---

## 8. Plan

```yaml
tasks:
  - title: Documentation site
    ref: docsite
    desc: |
      Hextra-on-Hugo documentation site at https://bensyverson.com/documentation/Jobs/.
      Replaces DOCS.md as the single source of truth for agent and human readers.
    labels: [docs]
    children:
      - title: Scaffolding
        ref: scaffolding
        desc: |
          Site infrastructure: Hugo/Hextra setup, `job schema` generation pipeline,
          hero screenshot, and the eventual DOCS.md retirement once content lands.
        labels: [docs, scaffolding]
        children:
          - title: Scaffold the Hugo + Hextra site
            ref: scaffold
            desc: |
              Mirror organizize/docs/. Create docs/hugo.yaml, docs/go.mod pulling Hextra as a
              module, docs/content/docs/_index.md with the cards landing, and a Makefile
              target `make docs` that runs `hugo serve` from docs/.
            labels: [docs, scaffolding]
            criteria:
              - hugo.yaml has baseURL https://bensyverson.com/documentation/Jobs/ and Hextra menu config
              - go.mod imports github.com/imfing/hextra
              - "`make docs` serves the empty site locally"
              - landing page renders with section cards (placeholders are fine)
          - title: Wire up `job schema` generation
            ref: schemagen
            desc: |
              `make docs-schema` runs `job schema > docs/content/docs/plan-grammar/_schema.json`.
              `make docs` depends on `docs-schema`. Commit the generated file so GitHub renders
              it without a build.
            labels: [docs, scaffolding]
            blockedBy: [scaffold]
            criteria:
              - make docs-schema regenerates _schema.json from the current binary
              - schema.md renders the JSON inside a fenced block via Hugo partial or shortcode
              - banner above the block reads "Generated from `job schema` — do not edit"
          - title: Hero screenshot
            ref: hero
            desc: |
              One dashboard screenshot used on both the docs landing page and the README.
              Capture from `job serve` against a representative .jobs.db.
            labels: [docs, scaffolding]
            blockedBy: [scaffold]
            criteria:
              - screenshot saved to docs/content/docs/hero.png (or similar)
              - landing page _index.md embeds it above the section cards
              - README.md placeholder replaced with the same image
          - title: Retire DOCS.md
            ref: retire
            desc: |
              Delete DOCS.md. Update any in-repo references (README, AGENTS.md, code comments)
              to point at the docs site or the appropriate page.
            labels: [docs, scaffolding]
            blockedBy: [gs, concepts, refsection, plansection, machine, webpage, recipes, contributing]
            criteria:
              - DOCS.md no longer exists
              - no remaining in-repo links to DOCS.md
              - README links point at docs/ section pages where appropriate
      - title: Content
        ref: content
        desc: |
          The eight authored sections of the site. Concepts and Reference are terse;
          Recipes is the only narrative section. Pages target agents first, humans second.
        labels: [docs, content]
        children:
          - title: Getting started section
            ref: gs
            desc: |
              Three pages — install.md, initialize.md, first-plan.md — plus _index.md.
              first-plan.md walks author → import → claim → done end-to-end.
            labels: [docs, content]
            blockedBy: [scaffold]
            criteria:
              - install.md covers `go install`, $GOBIN, PATH, AGENTS.md line
              - initialize.md covers `job init`, --default-identity, --strict, --gitignore
              - first-plan.md is a working walkthrough an agent could replay verbatim
          - title: Concepts section
            ref: concepts
            desc: |
              Six tight pages — identity, leaves-and-claims, criteria, blockers, labels, events.
              Each ~half a screen, agent-optimized, no narrative padding.
            labels: [docs, content]
            blockedBy: [scaffold]
            criteria:
              - identity.md explains --as, default identity, strict mode, attribution
              - leaves-and-claims.md covers the leaf frontier, TTL, auto-extend, auto-release, auto-close
              - criteria.md covers lifecycle, short_ids, --criterion / --all-passed / --force-close-with-pending
              - blockers.md covers block add/remove, cycle detection, auto-unblock
              - labels.md covers labels and the `decision` convention
              - events.md explains the append-only event log
          - title: Command reference section
            ref: refsection
            desc: |
              Five grouped pages — setup, planning, execution, observation, web — matching
              the `job --help` grouping. Goes one level above --help: idiomatic flags,
              edge cases, combinations, features that didn't make the base help.
            labels: [docs, content]
            blockedBy: [scaffold]
            criteria:
              - each page lists its verbs and the non-obvious flags worth knowing
              - no page repeats `job --help` output verbatim
              - features absent from base --help (e.g. -m @path, --claim-next races, --include-parents) are surfaced
          - title: Plan grammar section
            ref: plansection
            desc: |
              _index.md walks the YAML import format with worked examples (refs, blockedBy,
              children, labels, criteria). schema.md renders the generated JSON partial.
            labels: [docs, content]
            blockedBy: [schemagen]
            criteria:
              - _index.md shows a complete plan with refs, blockedBy, criteria, labels
              - schema.md renders the live `job schema` output via partial
          - title: Machine interface section
            ref: machine
            desc: |
              Two pages — json-output.md (CLI --format=json across read verbs, tail JSON-lines)
              and http-api.md (/events SSE + JSON replay).
            labels: [docs, content]
            blockedBy: [scaffold]
            criteria:
              - json-output.md enumerates which verbs accept --format=json and the shape returned
              - http-api.md documents /events query params, SSE framing, JSON event shape
              - both pages include curl + jq examples an agent could run unchanged
          - title: Web dashboard page
            ref: webpage
            desc: |
              Single brief page. What `job serve` does, who it's for, hero screenshot reused.
              Not a tutorial — humans figure dashboards out by clicking.
            labels: [docs, content]
            blockedBy: [hero]
            criteria:
              - page describes Home / Log / Tasks / Actors views in one line each
              - serve flags (--bind, --db) covered
              - links to /events API page rather than duplicating it
          - title: Recipes section
            ref: recipes
            desc: |
              Four narrative pages — great-plans, criteria-as-tests, multi-agent, recovery.
              The only place narrative belongs.
            labels: [docs, content]
            blockedBy: [concepts]
            criteria:
              - great-plans.md gives concrete advice on YAML authoring (sizing, refs, blockedBy hygiene)
              - criteria-as-tests.md frames criteria as first-draft unit tests with a worked example
              - multi-agent.md covers named identities, --as discipline, strict-mode bracketing for delegation
              - recovery.md covers reopen, cancel -m, cancel --purge — when to use which
          - title: Contributing page
            ref: contributing
            desc: |
              Port the "For contributors" content from DOCS.md — package layout, migrations,
              test helpers, pre-commit hooks.
            labels: [docs, content]
            blockedBy: [scaffold]
            criteria:
              - package layout (cmd/job, internal/job, internal/migrations, internal/web) covered
              - migration authoring rules covered
              - core.hooksPath setup covered
```