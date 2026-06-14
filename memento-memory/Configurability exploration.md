---
title: Configurability exploration
status: proposal
mode: append-only
date: 2026-06-14
tags:
  - memento
  - configuration
  - okf
  - lifecycle
  - proposal
  - open-question
summary: A catalogue of memento behaviours that have surfaced as potentially configurable — deployment format, edit-window policy, and future axes — without committing to expose any of them as user-facing config. Each entry records the default, the alternative use cases, the exposure shape, the cost of exposing, and the trigger that would justify implementation.
---

# Configurability exploration

## What this is

A catalogue of dimensions along which memento's behaviour could be made configurable, kept separate from the ADRs that pin defaults. The point is to keep the *space of plausible configurability* alive without pre-building any of it. The cost of recording the option is low; the cost of expressing it as code is high — every future feature acquires an extra axis to think about.

Each entry has the same shape:

- **Default** — what ships today, and which ADR pins it.
- **Alternative use cases** — who would want something else, and why.
- **Exposure shape** — what the configurability surface would plausibly look like (config flag, frontmatter field, both).
- **Cost of exposing** — what becomes more complicated for everyone, not just users of the alternative.
- **Trigger for implementation** — the evidence or user signal that would justify building it.

Append new threads below as further axes emerge. Do not edit prior threads in place; if a thread evolves, append a dated revision.

## Thread 1 — deployment format (Obsidian-aligned vs OKF-aligned)

**Default:** Obsidian-aligned. Wikilinks (`[[target]]`) as link syntax; `.md`-suffixed concept IDs in manifest keys; `index.md` / `log.md` as valid concept notes; bare-frontmatter tolerance. Pinned by ADR-0001 (vault naming), ADR-0007 (key stability), ADR-0005 (init / bare-markdown adoption), and the cheap-subset adoption in ADR-0018.

**Alternative use cases:**

- Teams publishing OKF bundles externally who want memento as their *editing* layer over those bundles, not just a producer-on-export.
- Non-Obsidian deployments (VS Code, static-site renderers, web viewers, agent-only consumption) where wikilinks are friction rather than feature.
- Organisations standardised on OKF as their cross-system interchange format.

**Exposure shape:** A `format:` flag in `.memento/config.toml`, set at `memento init` time. Plausible values `obsidian` (default) and `okf`. Affects link syntax in the body, key shape in the manifest, reserved-filename handling, and bare-markdown `type:` synthesis.

**Cost of exposing:** Real and structural. Once `format:` exists in config, every future design decision branches on it — manifest schema, brief rendering, write semantics, link-graph computation, error messages. Optionality has gravity. The "every future feature has a two-axis answer" cost is the one to weigh hardest.

**Cheap constraints already adopted** (no `format:` toggle needed today, but they keep the option alive):

- Typed-link information lives in frontmatter and the compiled manifest only, never in markdown body link syntax. Pinned in [[OKF interop and external compatibility]].
- Manifest schema is link-syntax-agnostic — stores resolved keys and typed-link metadata, not raw link text (spec §4).
- Memento-flavoured filenames live under `.memento/` and `_memento/` namespaces only. The vault root is left untouched, leaving `index.md` and `log.md` available for OKF reservation if dual-mode ever lands.
- ADR-0018 aligns the cheap subset of frontmatter conventions, narrowing the eventual cost of dual-mode.

**Trigger for implementation:** A concrete user surfaces with a non-Obsidian deployment intent, *or* an OKF consumer ecosystem emerges where bidirectional editing (not just export) is the natural shape, *or* OKF v0.2+ tightens compatibility expectations.

**Related:** [[OKF interop and external compatibility]] (full alignment analysis, addendum on dual-mode posture), ADR-0018 (OKF-compatible frontmatter conventions).

## Thread 2 — edit-window policy for new documents

**Default:** Per ADR-0017, the edit window is bounded by the *first commit*. While a file is uncommitted, the write gate accepts full-file rewrites regardless of declared mode. Once committed, the declared mode binds. Rule applies uniformly across `append-only`, `living`, and `read-only`.

**Alternative configurability axes worth holding in mind:**

- **Per-mode override.** A user could want `read-only` to bind immediately on first write (treat the declared mode as authoritative from creation), while keeping the standard pre-commit edit window for `append-only` and `living`. Use case: an organisation that treats `read-only` as a "this is truth from the moment it exists" assertion rather than "this is settled after commit." Cost: per-mode predicate logic on the write gate.
- **Commit-count boundary.** The default ratification is "first commit". An alternative is "first N commits", or "until tagged", or "until a `status:` field transitions." Use case: multi-commit drafting sessions where the agent wants to commit incrementally without losing the edit window. Cost: a parameter or status-aware predicate; introduces a notion of authorial state the tool currently does not have.
- **Time-bounded window.** "Edit window is 24 hours after creation, regardless of commits." Cost: a wall-clock dependency in the write gate. Probably too cute; flagged for completeness.
- **Authorship-bounded window.** "Only the originating session/agent can rewrite during the edit window; other agents see the mode bind immediately." Use case: multi-agent vaults where one agent's draft should not be freely rewritten by another. Cost: agent identity tracking, which memento explicitly avoids today.

**Exposure shape, if any:** A per-vault config block in `.memento/config.toml` (vault-wide policy) or per-doc frontmatter override (per-doc opt-in). The latter has the advantage of locality — the doc that wants special treatment carries the rule — but pulls against the principle that frontmatter declares content shape, not policy (ADR-0014). The two surfaces are not mutually exclusive.

**Cost of exposing:** Each axis above adds one branch to the write gate. The uniformity argument from ADR-0017 ("one predicate, one binding event, no per-mode arcana") weakens as soon as the knobs accumulate. The strongest move is to keep the default rule unconfigurable until concrete friction shows up, then expose the *one* axis the friction motivates — not the full space.

**Trigger for implementation:** Observed friction with the default rule. Specifically:

- Users routinely fighting the gate during multi-commit drafting sessions (would motivate the commit-count or status-aware axis).
- `read-only` docs being mutated in ways that subvert the supersede-don't-edit discipline (would motivate the per-mode-override axis).
- Multi-agent vault races (would motivate the authorship-bounded axis).

**Related:** ADR-0017 (default rule, ratification event, why session-bounded was rejected in favour of commit-bounded).

## Recording discipline

Entries above should follow the same shape. The shape is deliberately implementation-flavoured: the point of this doc is to record *what would change if we exposed the knob*, not to wonder whether a knob would be nice. If the cost cannot be articulated, the entry is not yet ready.

Append new threads below this line.
