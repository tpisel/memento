---
title: Init and pre-commit hook ownership
status: accepted
mode: read-only
date: 2026-06-11
tags:
  - memento
  - init
  - git
  - manifest
summary: " `init` installs or updates a sentinel-bounded block in `.git/hooks/pre-commit` (creating the hook if absent, preserving existing content if present). The hook runs manifest compilation and stages the resulting `.memento/manifest.json`. Re-running `init` is idempotent and never rewrites unrelated hook content."
---

# ADR-0005 - Init and pre-commit hook ownership

## Context

The manifest is committed so memory changes are visible in review and agents can load a static artifact with no build step. That creates a stale-manifest risk unless regeneration is integrated into normal git workflow.

Repositories may already have a pre-commit hook. Memento must be polite when installing automation and must not clobber existing user hook logic.

## Decision

`init` installs or updates a sentinel-bounded block in `.git/hooks/pre-commit`:

```sh
# memento:start
# ...
# memento:end
```

If the hook does not exist, `init` creates it with:

```sh
#!/bin/sh
set -eu
```

followed by the memento block.

If the hook exists, `init` appends the memento block if absent, or replaces only the content within the existing memento sentinels if present. It does not rewrite unrelated hook content.

The hook runs manifest compilation for the discovered memory vault and stages the resulting `.memento/manifest.json` when appropriate.

## Consequences

- Re-running `init` is idempotent.
- Existing hook behavior is preserved.
- The committed manifest remains the normal review surface for memory changes.
- Hook installation stays simple and local. Integration with hook managers can be added later if concrete demand appears.

