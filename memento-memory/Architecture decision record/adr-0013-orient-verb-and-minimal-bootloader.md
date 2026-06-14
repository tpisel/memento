---
title: Orient verb and minimal AGENTS.md bootloader
status: accepted
mode: read-only
date: 2026-06-14
tags:
  - memento
  - agents
  - bootloader
  - orient
summary: "`memento orient` becomes the preferred surface for telling agents how to use the tool. The AGENTS.md / CLAUDE.md block becomes a one-paragraph pointer set at `init` time and not actively managed afterwards. Orient output layers a code-internal baseline (verbs, semantics, triggered preconditions) with a user-curated overlay of docs frontmatter-tagged `orient: true`. Baseline first, user docs alphabetically by key."
---

# ADR-0013 — Orient verb and minimal AGENTS.md bootloader

## Decision

`memento orient` becomes the canonical surface for tool-usage instructions, replacing the content-bearing AGENTS.md / CLAUDE.md block prescribed in spec §11.

Concretely:

- **AGENTS.md insertion is minimal and inert after init.** The sentinel-bounded block contains roughly one paragraph: "Durable project knowledge lives in `<vault>`. Run `memento orient` for verbs and conventions, then `memento brief` to scan entries." It is written at `init` time and **not updated by memento afterwards**. Re-running `init` still replaces it (idempotency preserved per spec §11), but routine memento operations never touch AGENTS.md.
- **`memento orient` is THE preferred way to expose tool-usage instructions to agents.** New triggered preconditions, mode summaries, and verb additions land in orient — not in AGENTS.md.
- **Orient output is two-layered:**
  1. A **code-internal baseline** that ships with the binary. Not user-editable. Contents: verb list with one-line semantics, write-mode summary, triggered preconditions ("before X, do Y" — e.g., "before first `memento write` of a session, read the writing guide"), and pointers to `brief` and `read`.
  2. A **user-curated overlay**: any vault doc with `orient: true` in its frontmatter is appended to orient output in full. Single-tenant, Obsidian-browsable, edited like any other note.
- **Assembly order:** baseline first, then user-tagged docs sorted alphabetically by manifest key. Baseline is foundational; user docs refine or contextualise.
- **Naming:** the verb is `memento orient`, deliberately distinct from beads' `bd prime` (which is the memory-dump pattern memento §1 explicitly rejects).

## Context

Spec §11 currently has memento managing a sentinel-bounded content-bearing block in AGENTS.md. That worked when the block carried two lines (brief discoverability + retrieval instruction). It does not scale.

Two pressures pushed against keeping content in AGENTS.md:

1. **The block grows.** New conventions ("before writing to an ADR, do X"; "writing modes are append-only by default"; future verbs) each want a slice. AGENTS.md is also multi-tenant — beads has a claim, the project has a claim, memento has a claim. Contention compounds.
2. **Triggered preconditions can't be enforced at action time.** If `memento write` blocks first-call-in-session and tells the agent "read the writing guide first," the agent has already composed a write payload and the flow is wasted-work-then-revise. Preconditions must surface at orientation time — *before* the agent starts composing. That requires a session-start surface that isn't AGENTS.md (which is also session-start but cannot grow).

Three alternatives were considered and rejected:

- **Keep everything in AGENTS.md (status quo, spec §11).** Rejected: see above. The block becomes a swamp; updates require re-init; multi-tool contention is unresolved.
- **`memento prime` as a brief-style dump.** Rejected: this is the bd-prime pattern memento §1 explicitly rejects ("naive full dump dressed as retrieval"). If `prime` expands into a content dump, it re-introduces the failure mode the design exists to avoid.
- **Enforce preconditions inside each verb (e.g., `write` returns the writing guide on first call).** Rejected: the agent has already done planning/composition work by the time it invokes the verb. Returning rules at action time means rework, not flow.

Orient sidesteps the bd-prime critique because it is **bounded by intent**, not by accreted memory. It is the operating manual for the tool, not a dump of everything ever written down. The baseline is small and curated; the overlay is opt-in via an explicit frontmatter field, not by-default.

## Consequences

- The AGENTS.md slice memento owns stays small and stable. Memento's churn on shared instruction files drops to ~zero between `init` calls.
- Orient output is version-locked to the tool binary. Conventions added in a new memento release ship in the next orient call; the user does not have to re-init to pick them up.
- The user-overlay layer gives humans a clean, single-tenant home for project-specific orientation. Single file or several, all opt-in via `orient: true`. Obsidian-browsable; edited like any other note.
- One additional round-trip at session start (agent reads AGENTS.md → runs `memento orient` → optionally runs `memento brief`). The cost is small and bounded; the layering benefit is large.
- The verb name `orient` is reserved by memento. Future tool conventions ("auto-load this on write") get their own verbs or extend orient — they do not co-opt AGENTS.md.
- Spec §11 is refined, not superseded: the bootloader block still exists, is still sentinel-bounded, is still set by `init`. Its contents shrink to a pointer; its role shifts from carrier to signpost.

## Versioning

`memento orient` is a v2-or-later verb alongside the broader writing-conventions work. v0 keeps the spec §11 pattern as-is. The ADR pins the shape so v2 work does not re-litigate it.

The AGENTS.md minimisation can land independently of orient — even before the verb exists, the block can be a pointer to "memento brief and the eventual orient verb" — but in practice it is cleanest to land them together once `orient` is implemented.

## Open questions

- **Soft cap on combined orient size.** Orient output combines baseline + N user docs. There is no enforced size cap. Worth observing in practice; a warning at some token threshold may be appropriate later.
- **User-overlay ordering override.** Alphabetical-by-key is the v0 default. If real use shows users want explicit ordering (e.g., a high-priority orientation doc that must appear first), a numeric `orient: 10` priority field is the obvious next step. Defer.
- **Baseline inspection.** `memento orient --show-baseline` or similar for debugging / auditing the binary-shipped baseline. Useful but not required for v2.
- **Cross-tool orientation.** "Memento + beads + project form a system, here is how they relate" is a project-level concern, not memento's. Out of scope for this ADR; flagged as a problem each project's AGENTS.md must solve on its own.
