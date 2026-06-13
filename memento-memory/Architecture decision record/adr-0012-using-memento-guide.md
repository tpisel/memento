---
title: Init-scaffolded Using Memento guide
status: accepted
mode: read-only
date: 2026-06-13
tags: [init, memento, namespace, vault]
summary: "Replace the sentinel-managed `_memento/README.md` convention note with a one-shot `_memento/Using Memento.md` onboarding guide. The guide has no `mode: read-only` frontmatter or managed block, is fully user-owned after init writes it, and is ignored by compile so it does not appear in the manifest or brief."
---

# ADR-0012 - Init-scaffolded Using Memento guide

## Decision

`memento init` writes `_memento/Using Memento.md` when the file is absent. It is a humane onboarding note for someone opening the vault in Obsidian and wondering what `_memento/` is for.

The file is write-once:

- no `mode: read-only` frontmatter;
- no sentinel-bounded managed block;
- no re-render or update on later init runs;
- no migration or deletion of older `_memento/README.md` files.

Init also adds `_memento/Using Memento.md` to `.mementoignore`. The guide is a human onboarding artifact, not vault content, so compile must not surface it in `manifest.json` or `brief.md`.

## Context

ADR-0009 established `_memento/` as the visible namespace for human-readable tool artifacts and anticipated a convention README. Dogfooding showed that the sentinel-managed README felt like tool machinery in a place that should welcome a new user.

The better contract is simpler: init leaves a friendly starter note, then ownership transfers completely to the user. If they edit or delete it, memento should not fight them.

## Consequences

`_memento/brief.md` remains generated and ignored as before. Future tool-read convention files such as `_memento/writing.md`, `_memento/review.md`, and `_memento/audit.md` remain deferred to their corresponding verbs.

Existing `_memento/README.md` files from the earlier scaffold are user files now. Memento does not migrate them, modify them, or delete them.
