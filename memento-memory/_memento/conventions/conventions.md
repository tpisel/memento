---
title: Writing conventions
when_to_read: before adding or editing a memento convention
---

# Writing conventions

Conventions are operational guidance, not normal project-memory notes. They live under `_memento/conventions/`, are discovered from `when_to_read:` frontmatter, and are read through the convention mechanism when a workflow reaches the named circumstance.

## File naming

- Use a short, lowercase stem with no spaces, for example `writing.md`, `summarising.md`, or `beads.md`.
- Use hyphens only when a single word is unclear.
- Name the convention for the workflow or object it governs, not for an implementation detail.

## Frontmatter

Use only the fields needed by the convention mechanism unless a future ADR expands the schema:

```yaml
---
title: Human-readable convention title
when_to_read: before doing the workflow this convention governs
---
```

`when_to_read:` is required and must be non-empty. It should complete the sentence "Read this convention ...". Do not encode follow-up actions in frontmatter; put workflow instructions in the body.

Do not add normal note fields such as `summary`, `tags`, or `mode` just to satisfy brief/index expectations. Conventions are outside the normal brief corpus.

## Body

Keep the body operational and direct. State what the agent should do, what it should avoid, and what shape a valid result has. If the convention becomes highly opinionated for a project, prefer a template or project-specific convention over expanding the generic default.
