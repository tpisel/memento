# Agent Instructions

This project is being developed with two separate memory substrates:

- **beads** holds task state, implementation sequencing, blockers, and working notes.
- **memento memory** holds durable project knowledge: design decisions, specs, constraints, and discoveries that should survive a task.

Keep that boundary clear. Task progress belongs in beads. Durable semantic knowledge belongs in `memento-memory/`.

## Start of Task

1. Run `bd ready` and choose the next ready task unless the user has already named a task.
2. Read the selected task and any linked context before editing code.
3. Check `memento-memory/` for relevant durable context:
   - start with `memento-memory/spec.md`;
   - read ADRs linked from the task;
   - scan other ADRs when the task touches architecture, storage layout, CLI behavior, or agent workflow.
4. Keep the task loop small. Prefer finishing one beads task with tests and a close note over starting several partial threads.

When memento CLI support exists, replace the manual memory scan with the manifest/read workflow below.

<!-- memento:start -->
Durable project knowledge lives in `memento-memory/`.
The future manifest path is `memento-memory/.memento/manifest.json`.

Before a task: scan the manifest when present, using titles, summaries, tags, and headings to identify relevant entries. Read only the entries or sections that plausibly apply.

Until the CLI and manifest exist, manually read the relevant files in `memento-memory/`, especially `spec.md` and the Architecture decision record directory.

Working state lives in beads (`bd ready`). Discoveries that outlive a task go to `memento-memory/`, not beads close notes. Write back according to `memento-memory/writing_guide.md` once it exists.
<!-- memento:end -->

## During Implementation

- Let existing code and tests define the current behavior once code exists.
- Prefer small, deterministic slices with observable CLI behavior.
- Keep implementation choices aligned with the accepted ADRs.
- Do not store transient debugging notes in `memento-memory/`.
- If a task reveals a durable constraint, rejected alternative, or design correction, add or update a memory note or ADR in the same change set.

## End of Task

Before closing a beads task:

1. Run the relevant tests or checks.
2. Update the task with what changed, what was verified, and any remaining follow-up.
3. Move durable learnings into `memento-memory/` when they meet the writing threshold.
4. Leave beads close notes concise; do not turn them into long-term design docs.

## Current Implementation Plan

Initial work should be decomposed into beads tasks around these slices:

1. Scaffold Go module and CLI skeleton.
2. Implement vault discovery via `.memento/`.
3. Implement `.mementoignore` parser and matcher.
4. Implement markdown/frontmatter extraction.
5. Emit deterministic manifest.
6. Implement `read <key>`.
7. Implement `read <key>#<section>`.
8. Implement `init` adopt/create flow.
9. Install or update the pre-commit sentinel block.
10. Add minimal v0 write support for new files and append-only writes.

