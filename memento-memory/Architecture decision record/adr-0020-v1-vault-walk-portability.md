---
title: V1 vault walk portability policy
status: accepted
mode: read-only
date: 2026-06-14
tags: [memento, compile, portability, symlink, unicode, vault]
summary: V1 preserves filesystem-returned path spelling for manifest keys and skips all symlinks during vault content walks. NFC/NFD and case-only filename variants are distinct path spellings when the filesystem exposes them; symlink targets are never indexed through the link.
---

# ADR-0020 - V1 vault walk portability policy

## Context

V1 hardening needs the vault walk to behave predictably across the Linux and macOS CI matrix. Three portability edges matter early:

- Unicode filenames can be presented in composed or decomposed forms depending on how a filesystem stores and returns path names.
- Case-folded filesystems may refuse to represent two filenames that differ only by letter case.
- Symlinks can point outside the vault or back into already-walked trees, making traversal policy a safety and determinism question.

ADR-0003 deliberately left symlink traversal semantics at the "default safe filesystem walk" level for v0. V1 needs the policy to be explicit because manifest keys are the agent-facing address space.

## Decision

Manifest keys preserve the vault-relative path spelling returned by the filesystem walk. Memento does not canonicalize filenames to NFC or NFD, and does not case-fold keys, in v1. If a filesystem exposes both spellings as separate paths, both are separate manifest keys. If a filesystem normalizes or folds them before memento sees them, memento uses the returned spelling as the key.

The vault content walk skips all symlink entries. This includes symlinked files and symlinked directories, regardless of whether the target points inside or outside the vault. Only real markdown files reachable through ordinary directory traversal are indexed.

## Consequences

- The manifest stays aligned with the human-visible path space returned by the operating system and Obsidian-facing filesystem.
- Memento avoids indexing content outside the vault through a symlink and avoids cycles or duplicate entries through linked directories.
- Users who want linked content indexed must materialize it as ordinary files under the vault.
- Future canonical Unicode key matching can be considered only with a concrete user-facing miss; it would need a deliberate compatibility story because keys appear in manifests, briefs, beads references, and commit messages.
