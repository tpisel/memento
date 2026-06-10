---
title: Memory directory naming & vault topology
status: accepted
mode: read-only
date: 2026-06-10
tags: [memento, init, obsidian]
---

# ADR-0001 — Memory directory naming & vault topology

## Decision

The memory directory defaults to **`<project>-memory/`**, where `<project>` is derived at `init` time from the git remote basename, falling back to the repository directory name. If no project name can be derived, fall back to `memory/`.

`--dir` overrides the default. The bootloader block and manifest path always follow whatever directory `init` resolves; nothing hardcodes a fixed name.

**Only one memory vault per repository is supported.**

## Consequences

- Obsidian names a vault after its folder, so `<project>-memory/` keeps the vault switcher legible and avoids identical `memory` entries across projects. (Obsidian keys vaults by path, so same-named folders never collide on disk — this is a legibility fix, not a correctness one.)
- The human opens the resolved memory directory as the vault root, keeping the wikilink graph bounded to the memory store.
- Supersedes the `memory/` default recorded in spec §3; that section's "the name need not be descriptive" rationale no longer holds, since Obsidian conscripts the folder name into its UI.
