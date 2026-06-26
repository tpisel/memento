---
title: "_memento operational namespace and default ignore policy"
status: accepted
mode: read-only
date: 2026-06-26
tags:
  - memento
  - _memento
  - conventions
  - skills
  - brief
  - namespace
summary: "`_memento/` is reclassified as a wholly operational namespace rather than a mixed normal-memory folder. It is ignored from the normal manifest/brief corpus by default, with first-class operational access paths for generated brief output, convention files under `_memento/conventions/`, and agent skill projections under `_memento/skills/`."
---

# ADR-0030 - _memento operational namespace and default ignore policy

## Decision

`_memento/` is the vault's operational namespace, not part of the normal project-knowledge corpus. It should be ignored from the normal manifest and `brief` by default.

This amends ADR-0009's earlier "mixed-audience tool namespace" framing. The folder is still human-readable and may contain user-authored markdown, but its contents are consumed through workflow-specific operational surfaces rather than through ordinary `brief` browsing.

Default layout:

```text
_memento/
  brief.md                 # generated brief projection
  Using Memento.md          # human onboarding guide, user-owned after init
  conventions/
    writing.md              # when/how to write durable memento memory
    summarising.md          # how to write useful summaries
    conventions.md          # how to add or edit convention files
    beads.md                # optional project convention, if present
  skills/
    write.md                # agent skill projection/artifact, if present
```

Default ignore posture:

- `.mementoignore` should ignore `_memento/` as a normal note namespace.
- Memento-owned operational readers may still read specific `_memento/` paths directly.
- `brief` should not list `_memento/` files as ordinary notes.
- Generated files such as `_memento/brief.md` stay ignored; this ADR makes that rule structural rather than file-by-file accidental.

## Operational access paths

Different `_memento/` subtrees have different consumers:

- `_memento/brief.md` is the generated agent-facing projection of the normal manifest. It is consumed by `memento brief`, not by `memento read` as a normal note.
- `_memento/conventions/*.md` are conditional operational guides. They are discovered by `memento orient` through `when_to_read:` frontmatter and read with `memento convention <name>`.
- `_memento/skills/*.md` are agent skill projections or artifacts. Their installation and synchronization are agent-family-specific and remain distinct from normal memory retrieval.
- `_memento/Using Memento.md` is human onboarding prose. It is user-owned and ignored by the normal manifest.

Because these files are operational operands, they do not need to carry normal-note `summary`, `tags`, or `mode` frontmatter or appear in `memento brief`. If an operational file needs metadata, that metadata is for its operational reader. For conventions, the load-bearing metadata is `title:` and `when_to_read:`.

## Skills

Skills remain projections/artifacts, not canonical policy stores. A skill may point to or summarize a convention, but the convention file is the project-editable source of workflow policy. Installation should happen only when the backing source exists, and missing backing sources should fail clearly rather than causing the skill to invent behavior.

This ADR does not solve managed skill synchronization. It preserves ADR-0028's open questions: skill frontmatter compatibility, symlink direction, per-agent install paths, and whether skills become generated templates later.

## Context

The earlier `_memento/` design deliberately avoided treating the whole folder as generated or hidden because it needed space for human-readable tool documents. That was directionally right, but the normal manifest/brief surface proved to be the wrong discovery channel for those documents.

Operational guidance is conditional. Writing conventions are consumed when writing. Review conventions are consumed when reviewing. Skills are consumed through harness skill loading. The brief is itself generated as a brief. Listing all of that in normal `brief` blurs project knowledge with tool operation and pressures operational files to look like normal notes with summaries and tags.

ADR-0029 resolves conventions by giving them their own orient-discovered read path. This ADR resolves the containing namespace: `_memento/` is operational and ignored as a normal note source.

## Consequences

- Future init output should prefer `_memento/conventions/writing.md` over `_memento/writing.md`.
- `.mementoignore` defaults should move from selective `_memento/brief.md` and `_memento/Using Memento.md` ignores to a structural `_memento/` ignore, with operational readers bypassing normal note indexing as needed.
- Existing projects may need a migration path for `_memento/writing.md` and `_memento/skills/write.md` while the new convention and skill-install behavior lands.
- `memento brief` becomes cleaner: it lists project knowledge, not memento's own operational files.
- Operational readers must not rely on the normal manifest containing `_memento/` entries. They need their own lookup or operational index.

## Related

- [[adr-0009-memento-subfolder-namespace]] - amended by this ADR; `_memento/` remains the tool namespace, but no longer participates in normal brief/index by default.
- [[adr-0029-convention-files-and-convention-verb]] - defines `_memento/conventions/` and `memento convention <name>`.
- [[adr-0028-obsidian-managed-skills]] - keeps skill management questions deferred.
- [[adr-0008-memento-brief-projection]] - `_memento/brief.md` remains generated operational output.
