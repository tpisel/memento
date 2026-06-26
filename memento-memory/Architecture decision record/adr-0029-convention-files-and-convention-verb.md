---
title: "Convention files and the convention verb"
status: accepted
mode: read-only
date: 2026-06-26
tags:
  - memento
  - agents
  - conventions
  - orient
  - _memento
  - cli
summary: "Convention files move workflow guidance out of the normal brief corpus and into `_memento/conventions/`, where each valid convention declares only `title` and non-empty `when_to_read:` frontmatter. `memento orient` lists every valid convention as a conditional read prompt, and `memento convention <name>` reads the body without frontmatter. No hidden conventions: missing or empty `when_to_read` makes a convention invalid."
---

# ADR-0029 - Convention files and the convention verb

## Decision

Add a dedicated convention-file mechanism for conditional operational guidance.

A convention is a markdown file under `_memento/conventions/` whose frontmatter declares a non-empty `when_to_read:` string. The field names the circumstance in which an agent should read the file. The only supported action is to read the declaring file; follow-up behavior belongs in the body prose, not in a general if-then frontmatter language.

Example:

```yaml
---
title: Writing guide
when_to_read: before authoring a memento vault write
---
```

Convention frontmatter is intentionally smaller than normal note frontmatter. Do not add `mode`, `summary`, or `tags` to conventions by default. They are outside the normal brief corpus and do not need normal-note metadata to be useful.

`memento orient` renders a compact block from valid conventions:

```text
## When To Read Conventions

- Before authoring a memento vault write: `memento convention writing`
```

`memento convention <name>` reads `_memento/conventions/<name>.md`, strips frontmatter, and prints the body. It is the canonical access path for convention content. `memento read <key>` remains the canonical access path for normal memory notes, but conventions are operational guidance rather than project knowledge and should not need to appear in `brief` or satisfy normal note-summary expectations.

A convention file without `when_to_read:` is invalid. There is no supported hidden convention concept. Because every valid convention appears in orient, orient is also the convention discovery surface.

## Authoring conventions

There is no dedicated creation verb in this ADR. An agent adding or editing a convention must know the convention contract:

- Place conventions under `_memento/conventions/`.
- Use a short lowercase filename stem with no spaces, such as `writing.md`, `summarising.md`, or `beads.md`.
- Use hyphens only when a single word is unclear.
- Include `title:` and a non-empty `when_to_read:` in frontmatter.
- Make `when_to_read:` complete the sentence "Read this convention ...".
- Put workflow instructions in the body, not in frontmatter.
- Do not add normal note fields only to satisfy `brief`; conventions are operational and should not appear in the normal brief corpus.

The default conventions include a `conventions` convention whose body carries this authoring guidance so agents can load it before adding or revising convention files.

## Init templates

`init` creates minimal, generic convention templates under `_memento/conventions/`:

- `writing.md` - when to write durable memento memory and what belongs there.
- `summarising.md` - how to write useful summaries for retrieval.
- `conventions.md` - how to add or edit convention files.

These templates should stay project-neutral. More opinionated convention sets, such as conventions that assume beads or a specific engineering workflow, belong to future template support rather than default init.

## Validation and errors

- `memento orient` lists valid conventions and warns about invalid convention files.
- `memento convention <name>` fails with a convention-specific not-found error when `_memento/conventions/<name>.md` does not exist.
- `memento convention <name>` fails with an invalid-convention error when the file exists but has missing or empty `when_to_read:`.
- Compile may warn about invalid conventions, but should not fail the normal memory manifest solely because a convention file is malformed.
- The future `doctor` verb should report malformed conventions as operational health issues. `doctor` will have its own ADR; this ADR only records that malformed conventions belong in its remit.

## Context

ADR-0013 put triggered preconditions in `orient`, and ADR-0010 established `_memento/writing.md` as a project-local writing guide. In practice, the writing guide is operational content: it should be loaded when writing, not treated as an ordinary durable project-knowledge note during brief browsing.

Generalizing the old hard-coded writing-guide line into `when_to_read:` keeps the context-injection discipline intact. Orient injects only the small conditional pointer. The body is still pulled only when the triggering workflow starts.

A general frontmatter shape such as `precondition: { trigger, read }` was considered and rejected. It implies a broader action language than we need. The supported model is simpler: if a convention says when to read it, orient tells the agent when to read it.

## Consequences

- Convention files are conditionally discoverable through orient, not part of the normal brief corpus.
- All conventions must be named by their file stem. For now, aliases and alternate paths are out of scope.
- `memento convention <name>` can strip frontmatter and produce cleaner errors than `memento read` because it knows it is reading operational guidance.
- `_memento/writing.md` is superseded by `_memento/conventions/writing.md` for new init output. A compatibility migration path may keep reading the old path for a release, but new generated text should point to the new convention path.
- Convention aliases such as `memento read --convention writing`, header-stripping options on `read`, and a convention listing verb are deferred until real usage shows friction.

## Related

- [[adr-0010-tool-read-writing-guide]] - originally pinned `_memento/writing.md` as the write guide.
- [[adr-0013-orient-verb-and-minimal-bootloader]] - orient is the triggered-precondition surface.
- [[adr-0024-bootloader-contents]] - bootloader stays a signpost; convention pointers belong in orient.
- [[adr-0030-memento-operational-namespace]] - defines `_memento/` as operational and ignored by the normal brief corpus.
