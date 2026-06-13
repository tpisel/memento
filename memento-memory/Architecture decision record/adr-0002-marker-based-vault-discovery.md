---
title: Marker-based vault discovery and operational layout
status: accepted
mode: read-only
date: 2026-06-11
tags: [memento, init, manifest, config, obsidian]
summary: Vault discovery is by `.memento/` marker directory, not by directory name — exactly one marker per repository, ambiguity is a hard error. Tool-owned operational files (`config.toml`, `manifest.json`) live under `.memento/`; `.mementoignore` stays at the memory root because it describes the content namespace.
---

# ADR-0002 - Marker-based vault discovery and operational layout

## Context

ADR-0001 chose a descriptive default vault directory name so Obsidian's vault switcher remains legible. That solved the default naming problem, but the tool still needs a durable way to discover the memory vault after creation. Coupling discovery to a naming convention would make later renames and project-specific folder names unnecessarily fragile.

Operational files also need a home. The manifest and config are tool-owned artifacts, not memory content. Keeping them at the vault root would mix tool machinery into the human-authored note namespace.

## Decision

Memento discovers the repository memory vault by finding a directory named `.memento/` inside a candidate memory directory:

```text
<repo>/<memory-dir>/.memento/
```

Exactly one marker directory is supported per repository. If discovery finds more than one `.memento/` marker under the repository, the command fails and asks the user to resolve the ambiguity. If none is found, commands that require an existing vault fail unless the user provides an explicit directory or is running `init`.

`init` still creates or adopts a descriptive default directory, currently `<project>-memory/`, but the resulting vault is discovered by marker presence, not by directory name.

Tool-owned operational files live under the marker directory:

```text
<memory-dir>/
  .memento/
    config.toml
    manifest.json
  .mementoignore
  ...
```

The ignore file remains at the memory root because it describes the content namespace. The manifest path used by the bootloader is therefore:

```text
<memory-dir>/.memento/manifest.json
```

Per-user or machine-wide defaults in `~/.config/memento/` are deferred.

## Consequences

- A vault can be renamed without breaking discovery, as long as its `.memento/` marker moves with it.
- Obsidian-visible naming remains human-friendly while tool discovery remains explicit.
- The manifest and config are hidden from Obsidian's normal browsing surface.
- The repository supports one memory vault. This keeps bootloader instructions, manifest paths, and agent behavior unambiguous.
- Ambiguous discovery is a hard error, not a heuristic choice.

