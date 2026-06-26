---
title: Writing memento memory
when_to_read: before authoring or editing durable memento memory
---

# Writing memento memory

Write memento notes for project knowledge that should survive a task and is not obvious from the code. Keep task state, sequencing, blockers, and close notes in the task store; keep this vault for design decisions, durable constraints, and discoveries a future implementation loop should inherit.

## Write when

- A task reveals a constraint that is not visible in code, tests, or accepted ADRs.
- A decision is made with non-obvious rationale, rejected alternatives, or scope boundaries. Prefer a new ADR for accepted architecture or workflow decisions.
- Existing durable memory is now misleading and future agents would make worse choices if it stayed uncorrected.
- Dogfooding changes the intended agent workflow, storage layout, CLI contract, or memento/task-store boundary.

## Do not write

- Do not record transient task progress, debugging trails, command output, or "what I tried" notes. Put those in the task store when they matter.
- Do not restate behavior already encoded clearly in the implementation or tests.
- Do not turn task close notes into durable design docs; extract only the part that changes project understanding.
- Do not guess. If evidence is thin, leave a task comment or an open-question note instead of ratifying it as fact.

## Shape

Keep durable notes short enough to scan from `memento brief`. Summaries should lead with the load-bearing fact or decision. ADRs are for accepted architecture or workflow decisions; evolving reference notes are for material expected to keep changing.
