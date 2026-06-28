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

## Frontmatter

Every durable note carries this frontmatter. Copy the template and fill it in; do not reconstruct the schema from memory.

```yaml
---
title: Optional. Overrides the H1-derived manifest title; omit to use the H1.
summary: "Lead with the load-bearing fact or decision. This is the text memento brief shows."
tags:
  - one-or-more
  - lowercase-tags
mode: living
status: reference
date: 2026-06-26
---
```

- `title`, `summary`, `tags`, `mode` are **tool-consumed** (ADR-0014): memento reads them into the manifest and enforces `mode`. Spell them exactly — unknown keys are silently ignored, so `mod:` or `tag:` fails without warning.
- `mode` is one of `append-only`, `living`, `read-only` (ADR-0015). Absent `mode:` defaults to `append-only`. Use `living` for reference notes that evolve in place, `read-only` for frozen records, `append-only` for logs/journals.
- `status` and `date` are **convention** fields: memento parses but ignores them. They exist for human discipline and are not required by the tool. Keep `date` as `YYYY-MM-DD`.

## Shape

Keep durable notes short enough to scan from `memento brief`. Summaries should lead with the load-bearing fact or decision. ADRs are for accepted architecture or workflow decisions; evolving reference notes are for material expected to keep changing.

## How writes are enforced

There is no write verb — author notes with your native file tools (Write/Edit/Bash on Claude; `apply_patch` or shell on codex). A PreToolUse `check-write` hook enforces the note's `mode` before the bytes land: a ratified `read-only` note is refused, and an `append-only` note rejects anything that is not a pure append. This real-time gate covers Write/Edit/MultiEdit and recognisable Bash writes on Claude, and `apply_patch` on codex; **raw shell writes on codex are not gated in real time** (a codex-cli limit) and are instead caught after the fact by the commit-time mode audit, so on codex prefer `apply_patch` for vault notes. Modes bite only after a note's first commit, so first-draft authoring never walls, and new notes are created by a normal native write. A mode that blocks your write is a deliberate constraint, not an escape-hatch to open: stop rather than loosen it yourself. Loosening — `memento unlock` for a one-off authorised edit, `memento write-mode` for a permanent change — needs the user's explicit say-so; being asked to do the task is not authorisation to loosen the note. This covers `append-only` overwrites as much as `read-only` thaws: surface the block as a permission blocker and re-confirm before loosening, even when the instruction seems to imply the change.
