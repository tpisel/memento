---
title: "Convention files are the writable, always-living exception in the operational namespace"
summary: "_memento/conventions/<stem>.md is the one writable carve-out inside the otherwise-rejected operational namespace: the write gate admits it (bypassing the blanket _memento/ and ignored-path rejections) and treats conventions as living so a committed convention can be revised. This reconciles the enforcement layer with ADR-0029/0030, which call conventions agent-/project-editable. write-mode and unlock do not apply to conventions — they carry no mode: field."
tags:
  - memento
  - conventions
  - enforce
  - check-write
  - _memento
mode: living
status: reference
date: 2026-06-30
---

# Convention files are the writable, always-living exception in the operational namespace

ADR-0029 contemplates "an agent adding or editing a convention" and ADR-0030 calls the convention file "the project-editable source of workflow policy." But the write gate (ADR-0031) rejects the whole `_memento/` namespace twice over: `enforce.NormalizeWritableKey` rejects any path whose first segment is `vault.ToolDirName`, and `_memento/` is also a structurally ignored path (ignored paths are rejected too). So every gated agent tool-write to a convention was denied `unwritable_path`, contradicting the two accepted ADRs. This was memento-66t.

Resolution (option a — make conventions writable, not amend the ADRs to "human-maintained only"):

- `convention.IsConventionKey(key)` is the carve-out predicate: true only for `_memento/conventions/<valid-stem>.md` (exactly three segments, `.md`, stem passes `ValidateName`). It never matches a misfiled normal note (uppercase, spaces, nested path) dropped into the conventions dir, and never matches sibling operational subtrees (`_memento/skills/`, `_memento/brief.md`).
- `NormalizeWritableKey` returns convention keys early, ahead of both the blanket `_memento/` rejection and the ignored-path loop.
- A path carve-out alone is insufficient: conventions carry no `mode:` field (ADR-0029 forbids one), so a committed convention would default to `append-only` and reject any rewrite. `cli.effectiveMode` therefore resolves convention keys to `living`. The drive-by mode-change defense still runs and still blocks smuggling a `mode:` line into a convention.
- `write-mode` and `unlock` reject convention keys: conventions have no write-mode lattice to change or thaw, so both operations are meaningless on them (and `write-mode` would inject a forbidden `mode:` field).

Net effect: conventions are the single writable, always-living exception inside an otherwise-frozen operational namespace, editable by agents without loosening.

Related: [[adr-0029-convention-files-and-convention-verb]], [[adr-0030-memento-operational-namespace]], [[adr-0031-remove-write-verb-hook-enforced-native-writes]].
