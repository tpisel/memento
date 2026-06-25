---
title: "Obsidian-managed skills (thought-bubble) — skills as vault content"
status: draft
mode: living
date: 2026-06-25
tags:
  - memento
  - agents
  - skills
  - obsidian
  - portability
  - thought-bubble
summary: "Placeholder. Skills are markdown with frontmatter, and memento manages markdown with frontmatter — so memento could own skills as first-class, Obsidian-viewable vault notes, symlinked out into each harness's skills dir. Likely converges on _memento/skills/ (already seeded by ADR-0025). Two crux constraints noted: skill-frontmatter vs memento canonical frontmatter, and symlink direction vs ADR-0020. Not scoped; do not build."
---

# ADR-0028 — Obsidian-managed skills (thought-bubble)

> Status: draft / thought-bubble. Captured so the idea and its constraints are not lost. Not scoped, not scheduled.

## The thought

Skills are markdown + frontmatter; memento *is* a markdown-with-frontmatter manager. So a skill could be a first-class vault note: authored and browsed in Obsidian like any other memory, compiled into the manifest, and surfaced to the agent by symlinking it into the harness's skills directory. This extends memento's substrate thesis from *memory* to *capability* — one markdown source, two consumers (human via Obsidian, agent via the harness).

It also has a nice pedagogical payoff: the example write skill (`_memento/skills/write.md`, seeded by ADR-0025) becomes a human-visible, editable artifact — *"this is how memento is telling the agent to use me; I can change it."* A live operational example of the skills folder.

We will likely still converge on `_memento/skills/` as the home (consistent with ADR-0009's `_memento/` namespace).

## Two crux constraints (bank now)

1. **Frontmatter collision.** Harness skills require their own frontmatter schema (`name`, `description`); memento has a canonical vocabulary (ADR-0014) plus OKF compatibility (ADR-0018). Whether these reconcile — coexist, namespace, or project one from the other — *is* the crux of this ADR. ADR-0025 tolerates the raw skill frontmatter only because the file is neither compiled-as-content nor installed.

2. **Symlink direction vs ADR-0020.** The vault walk skips all symlinks. So the real file must live **in the vault** (compiled, Obsidian-visible) with the symlink pointing **out** into `.claude/skills/` (or the relevant harness dir). Symlinking *into* the vault would make the skill invisible to compile.

## Why deferred

- Pure additive future work, no v1 dependency.
- Distinct conceptual domain (capability packaging) from ADR-0025's priming-reliability problem.
- Gated behind the A-UAT outcome ([[adr-0026-agent-uat-validation-regime]]): if skills do not earn their keep, this management story is moot.

## Related

- [[adr-0025-agent-integration-orient-hook-and-write-skill]] — seeds `_memento/skills/write.md` as a plain artifact; this ADR would make it managed vault content.
- [[adr-0020-v1-vault-walk-portability]] — the symlink-skip policy that dictates link direction.
- [[adr-0014-canonical-frontmatter-vocabulary]] / [[adr-0018-okf-compatible-frontmatter]] — the frontmatter the skill schema must reconcile with.
- [[adr-0009-memento-subfolder-namespace]] — `_memento/` as the tool-artifact namespace.
