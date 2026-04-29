# Wire criteria into the Dashboard, clean up history rendering

```yaml
tasks:
  - title: Wire criteria into the Dashboard, clean up history rendering
    desc: |
      Two related dashboard cleanups discovered while dogfooding:

      1) The dashboard has no awareness of acceptance criteria. The data exists in the DB (migration 0003) but neither the task detail page, the peek sheet, nor the Log tab render it. The `criteria_added` and `criterion_state` events also fall through the dashboard's verb maps and emit raw snake_case ("criteria_added by alice") in History rows; they're absent from the Log filter chip bar entirely. The CLI's FormatEventDescription already has nice handling — there's a smell that web and CLI maintain two independent verb tables, but that consolidation is out of scope here.

      2) History rows inline the full note text after the verb, which duplicates the note body that lives elsewhere on the page. The task page exposes only the *completion* note above History, so progress notes from `noted` events live ONLY inside History rows today — meaning we can't just strip them without first surfacing them in their own section. The peek sheet additionally mis-labels its single completion-note block as "Notes" (plural, generic), conflating it with progress notes that aren't actually rendered.

      Render shape for criteria: a checklist between (Completion note / Progress notes) and History, four states — pending (empty box), passed (check), skipped (em-dash), failed (×). Match the CLI's `criterionGlyph` vocabulary so the two surfaces tell the same story.
    children:
      - title: Render criteria on the task detail page
        desc: |
          Add a "Criteria" section between the Notes/Completion-note block and History on /tasks/<id>. Renders as a checklist where each row shows the four-state glyph, the label, and (for non-pending) a quiet trailing state badge (passed / skipped / failed) so screen readers and color-blind users get the state without relying on the glyph alone. Section is omitted entirely when the task has zero criteria.
        criteria:
          - Section appears between completion-note (or progress-notes) and History
          - Pending rows render with an empty checkbox glyph
          - Passed rows render with a check glyph
          - Skipped rows render with an em-dash glyph
          - Failed rows render with an × glyph
          - Each non-pending row carries a state badge accessible to screen readers
          - Section omitted when task has zero criteria
          - Glyph vocabulary matches the CLI's criterionGlyph
      - title: Render criteria on the peek sheet
        desc: |
          Same Criteria section as the task detail page, sized for the peek sheet's narrower column. Same four-state glyphs, same trailing badges, same "omitted when zero criteria" rule. Sits between the completion-note section (once relabeled per the sibling task) and History.
        criteria:
          - Section appears in peek between completion-note and History
          - Same four-state glyph rendering as the task page
          - Section omitted when task has zero criteria
          - Visual weight tuned for the peek sheet's narrower column
      - title: Add criteria to the task-page data pipeline + JSON island
        desc: |
          Extend the task-detail handler in internal/web/handlers/tasks.go to load criteria for the rendered task, including state. Add the same to the JSON island so client-side replay can reflect criteria_added and criterion_state events without a server round-trip. Extend applyEvent / reverseEvent in the JS replay buffer to handle both event types (criteria_added appends entries; criterion_state mutates a row's state, with the prior state captured for invertibility). Peek handler gets the same data.
        criteria:
          - Task handler returns criteria with current state in its view model
          - Peek handler returns criteria with current state in its view model
          - JSON island includes criteria for the rendered task
          - applyEvent handles criteria_added (append) and criterion_state (mutate)
          - reverseEvent handles both event types using the same prior-state breadcrumb the events already carry
          - JS replay tests cover both forward and reverse paths
      - title: Map criteria_added and criterion_state to human-readable verbs
        desc: |
          The web layer's verb tables (tasks.go's eventVerb and log.go's VerbText assignment) currently fall through to raw event-type strings for criteria_added and criterion_state, leaking snake_case into History and Log rows. Add explicit cases that mirror the CLI's wording: "criteria_added" → "added N criteria, by"; "criterion_state" → "marked \"label\" passed" (substituting the actual label and state). Apply the same vocabulary to both task/peek History and the Log tab.
        criteria:
          - tasks.go eventVerb covers criteria_added with a count-aware phrase
          - tasks.go eventVerb covers criterion_state with the label and new state
          - log.go's row formatting covers criteria_added with the same phrase
          - log.go's row formatting covers criterion_state with the same phrase
          - No surface still emits the raw snake_case event type
      - title: Add criteria_added and criterion_state to the Log filter chip bar
        desc: |
          knownEventTypes in internal/web/handlers/log.go is the canonical ordered set of event types surfaced as filter chips. criteria_added and criterion_state are missing, so the Log tab can't filter to criterion activity. Append both, in the position that matches the prototype's intended layout (after edit/note adjacencies — they're authoring-side activity, not state transitions like done/canceled).
        criteria:
          - knownEventTypes includes criteria_added
          - knownEventTypes includes criterion_state
          - The chip ordering still reads coherently against the prototype
          - Filtering by either chip narrows the Log to that event type
      - title: Add a Progress-notes section above History on task and peek
        desc: |
          Progress notes (noted events) live ONLY inside History rows today. Before we can strip note text from History rows, we need to surface progress notes in their own section so they don't disappear from the UI. New section "Progress notes" sits above History, after Description and Completion note (and after Criteria once that lands). Each row: actor avatar, actor name, note text, relative time. Section omitted when the task has zero progress notes.
        criteria:
          - Section title reads "Progress notes" (plural)
          - One row per noted event, newest first
          - Each row shows actor avatar, actor name, note body, relative time
          - Section omitted when task has zero progress notes
          - Renders on both task detail page and peek sheet
      - title: Stop inlining note text into History rows
        desc: |
          Once Progress-notes section is live, drop the inline " — <note text>" trailer from History rows for both task/peek (extractNoteFromDetail) and Log (notePreviewFromDetail). Rows should read as "noted by <actor>" or "done by <actor>" alone — the actor + verb + timestamp is enough; the body lives in its dedicated section above. Hard-blocked on Progress-notes section landing first; without it, stripping noted-row text loses the body entirely.
        criteria:
          - extractNoteFromDetail no longer pulls note bodies into task/peek History
          - notePreviewFromDetail no longer pulls note bodies into Log rows
          - Done events render as "done by <actor>" with no inline note trailer
          - Noted events render as "noted by <actor>" with no inline note trailer
          - Cancel events render as "canceled by <actor>" with no inline note trailer
        blockedBy:
          - Add a Progress-notes section above History on task and peek
      - title: Rename the peek sheet's "Notes" section to "Completion note"
        desc: |
          The peek template renders the completion-note block under a section labeled "Notes", which conflates it with progress notes (which aren't actually rendered there). Rename the label to match the task page ("Completion note"). One-line change in internal/web/templates/html/pages/peek.html.tmpl.
        criteria:
          - Peek sheet section header reads "Completion note"
          - Renders only when the task has a completion note (existing condition unchanged)
          - Visual styling unchanged
```
