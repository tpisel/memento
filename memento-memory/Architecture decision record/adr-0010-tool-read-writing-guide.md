---
title: "Tool-read writing guide — _memento/writing.md, prose schema, orient-driven read"
status: accepted
mode: read-only
date: 2026-06-15
tags:
  - memento
  - write
  - orient
  - convention
summary: "Pin `_memento/writing.md` as the project-curated, prose-shaped writing guide for agents. It is optional; when present, `memento orient`'s description of `memento write` includes the precondition 'read _memento/writing.md before authoring'. The tool does not validate prose content, does not reinforce the instruction at write-time, and does not detect 'read this session' (stateless across invocations). Greenfield init scaffolds a minimal writing.md; adoption never clobbers."
---

# ADR-0010 — Tool-read writing guide: `_memento/writing.md`, prose schema, orient-driven read

## Decision

`_memento/writing.md` is the canonical project-curated writing guide. Per ADR-0009's deferred slot for tool-read filenames, it is the first such filename to be pinned.

Concretely:

- **Location: `<vault>/_memento/writing.md`.** Visible operational namespace (ADR-0009), Obsidian-browsable, human-curated, file-versioned by default. *Not* `<vault>/.memento/writing.md` — see Rejected alternatives.
- **Schema: free-form markdown prose, best-effort.** No required headings, no required frontmatter fields beyond standard memento conventions. The file is read by an LLM, not parsed. A soft convention is suggested but not enforced: `## Write when`, `## Do not write`, and per-doc-type formatting notes (e.g., ADR header conventions).
- **Optional.** Absence is supported and unremarkable; the tool emits no warning. Future `doctor`/`review` verbs may surface absence as a soft signal.
- **Mode: `read-only`.** It is a curated convention, not appended to. The pre-commit edit window (ADR-0017) still applies to drafting.
- **Agent surface (mechanism):**
  1. `memento orient`'s baseline describes `memento write`. When `_memento/writing.md` is present in the resolved vault, the orient description of `write` includes the precondition: *"before authoring, run `memento read _memento/writing.md`."*
  2. When absent, orient omits the precondition entirely — no broken pointer.
  3. `memento write` does *not* print a reinforcing reminder to stderr. The instruction lives in orient; reinforcement is the agent's responsibility.
  4. `memento write` does *not* gate on whether writing.md has been read this session. Memento is stateless across CLI invocations; detecting "read this session" requires either an agent-supplied session ID or local state, both of which were rejected for the ratification problem in ADR-0017 and are rejected here for the same reason.
- **Cleavage with orient and AGENTS.md bootloader notes:**

  | Surface | Owns | Loaded |
  |---|---|---|
  | Orient baseline (binary-shipped, ADR-0013) | Universal CLI verbs, semantics, triggered preconditions | Session start, by agent on bootloader instruction |
  | Orient overlay (`orient: true` notes) | Universal-but-project-specific orientation | Session start, alongside baseline |
  | Brief-loaded notes (e.g., `documentation-practices.md`) | "When to write an ADR" — high-level triggers | Session start, via brief scan |
  | `_memento/writing.md` | "If writing an ADR, use these headers/conventions" — configurable style and format | Lazily, only when an authoring step is imminent |

  Writing.md is **not** loaded into orient (no overlay flag honored). Its whole point is per-write lazy load — pulling it into orient would re-introduce the always-load failure mode the design exists to avoid.

- **`_memento/writing.md` is the *only* `_memento/` filename with a special agent flow today.** Other reserved names (`review.md`, `audit.md`) are placeholders; their flows will be pinned when their verbs land.
- **Greenfield init scaffolds a minimal writing.md.** Content is parsimonious good-practice prose: "retain hard-won learnings, paths we decided not to take," and similar. Canonical opinionated content (e.g., ADR conventions) is deferred to `init --template=` work per ADR-0009.
- **Adoption (non-empty vault) never clobbers.** If `_memento/writing.md` already exists, init leaves it. If absent, init does *not* create one during adoption — the human opts in.
- **No validation beyond standard frontmatter.** writing.md is parsed as any other markdown note: frontmatter checked strictly (ADR-0014), body left alone.

## Context

Spec §8 ("trigger-shaped writing rules ... read at write-time") and ADR-0009 (the `_memento/` namespace) both pointed at a tool-read writing guide whose filename and read mechanism were left to ADR-0010. Three design pressures shape the decision:

1. **The instruction has to reach the agent *before* it composes a write.** Returning rules at write-time means the agent has already done the planning and composition work the rules were meant to shape — rework, not flow. The same observation that drove ADR-0013 (orient over per-verb gating) drives this ADR: surface preconditions at orientation, not at action.

2. **The tool is stateless across CLI invocations.** Any mechanism that says "refuse to write if writing.md hasn't been read this session" needs durable state — agent-supplied session IDs, `.memento/in-flight.json`, etc. ADR-0017 rejected the same shape for ratification and the rejection generalises: stateless beats heuristic-state.

3. **Curation has to stay cheap, or the file rots.** A schema that forces particular sections or frontmatter fields raises authoring friction. A free-form prose file the human tunes when they notice the agent doing the wrong thing is the only shape that survives long-term curation.

The orient-driven read mechanism handles (1) by surfacing the precondition at session start, where the agent reads its operating manual. It handles (2) by avoiding statefulness entirely — orient just says "before authoring, read writing.md," and the agent's existing context tracks whether it has. It handles (3) by leaving the file unstructured.

### Why `_memento/writing.md`, not `.memento/writing.md`

`writing.md` is *human-authored*. The human writes triggers and style notes; the human tunes them when the agent does the wrong thing; the human may want to browse the file in Obsidian alongside other notes about the project. That argues for the visible operational namespace (`_memento/`).

The counter-argument considered: writing.md is "named-flow-special" — its filename is a contract with the tool, like `manifest.json` — and named-flow-special things might belong in the machine namespace (`.memento/`) for consistency. This was rejected because it conflates *mechanism specialness* (the tool reads this specific filename at a specific moment) with *storage specialness* (machine-owned binary/structured state). The mechanism is special; the storage is human-curated content. ADR-0009's framing already accounts for this: filename is the contract, the namespace is determined by audience. writing.md's audience is the human curator and the agent reading prose — `_memento/` fits.

### Why no in-tool reinforcement

The minimal expansion considered was a stderr reminder printed by `memento write` on every invocation: *"reminder: run `memento read _memento/writing.md` before authoring."* Rejected for the agent-flow reason: by the time the agent has invoked `write`, the planning/composition step is already done. Reinforcement at this point either lands too late to be actionable (the rules apply to what the agent has *already* composed) or, worse, trains the agent to skim stderr for status it will then ignore.

The more comprehensive expansion — refuse to write until writing.md has been read this session — was rejected as over-finessing, for the statelessness reason above and because it imposes a hard failure mode on what is fundamentally an advisory convention.

The orient-only mechanism is the simplest position that preserves agent flow.

### Why writing.md doesn't get an `orient: true` overlay

Writing.md *could* be tagged `orient: true` so it appears in orient output. This was rejected. Orient is the always-loaded surface (universal conventions); writing.md is the lazy surface (per-write style). Conflating them means an agent loads writing.md every session whether or not it intends to write, which is exactly the always-load failure mode spec §1 targets. The two surfaces stay distinct.

## Consequences

- Agents land in vaults with a writing.md and get a precondition pointer at session start. Agents land in vaults without one and operate from orient + brief + frontmatter conventions alone — fine, no degradation.
- The human authoring surface is one file in an obvious place. Editable in Obsidian, versioned in git, diffable in PR.
- The init flow on greenfield projects scaffolds a small writing.md as part of the convention-by-example pattern (spec §11, ADR-0009). Adoption flows do not.
- `memento orient`'s baseline gains a conditional fragment: the `memento write` description includes the precondition line iff `_memento/writing.md` exists at orient time. Implementation is a single existence check at orient-render time.
- Future tool-read files in `_memento/` (e.g., `review.md`, `audit.md`) follow the same pattern: optional, free-form prose, lazy read by their associated verb, no enforcement. This ADR sets the precedent.
- Spec §8 ("a tool-read writing guide ... read at write-time") and ADR-0009's deferred ADR-0010 slot are both resolved.

## Open questions

- **Precedence when multiple tool-read `_memento/*.md` files exist.** Today only writing.md is defined; precedence is moot. If review.md or audit.md later overlap conceptually with writing.md, this ADR or a sibling ADR will pin precedence.
- **Doctor/review surfacing.** A future `memento doctor` or `memento review` verb may flag missing writing.md as a soft signal ("this vault has no project writing conventions"). Out of scope here; deferred until those verbs land.
- **Soft size cap.** If writing.md grows past some threshold, an orient-time warning may be appropriate. No cap shipped; observe in dogfooding.
- **Whether absence of writing.md should change the orient precondition fragment to a positive nudge** (e.g., "consider writing one"). Lean no — keep orient terse; doctor handles this.

## Supersedes (partial)

- Spec §8 reference to "tool-read writing guide" — this ADR pins the filename, location, schema, and read mechanism. The supersession is additive; spec §8's framing of *when* the agent should write is unchanged.
- ADR-0009's deferred ADR-0010 slot — closed by this ADR for `writing.md`. Other tool-read filenames remain to be pinned in subsequent ADRs as their verbs land.

## Related

- [[adr-0009-memento-subfolder-namespace]] — `_memento/` namespace and the file-level ownership model that places writing.md there.
- [[adr-0013-orient-verb-and-minimal-bootloader]] — orient as the always-loaded baseline + overlay surface; this ADR adds the writing.md precondition fragment to orient's `write` description.
- [[adr-0015-write-mode-taxonomy]] — writing.md's mode is `read-only`; its content describes how to use the modes from ADR-0015.
- [[adr-0017-pre-commit-edit-window]] — edit-window still applies to writing.md drafting like any other note.

## Addendum 2026-06-17 — Tool-read file pattern (verb-paired prose conventions)

`writing.md` was the first tool-read file in `_memento/`. As `review.md`, `audit.md`, and subsequent verbs land, each will sit beside it under the same pattern. This addendum lifts the shape from precedent to contract, so each new verb's filename does not redesign the surface from scratch.

A **tool-read file** is a `_memento/<verb>.md` prose convention paired with a memento verb. It is the *pull* surface for per-action conventions, distinct from the orient overlay (the *push* surface for project-wide orientation). The two stay separate; conflating them re-introduces the always-load failure mode that orient itself was carved out to avoid.

The contract every tool-read file obeys:

1. **Location.** `<vault>/_memento/<verb>.md`. One filename per verb. The visible operational namespace (ADR-0009); Obsidian-browsable, file-versioned by default.
2. **Audience.** Human-curated, agent-read. The human tunes the prose when the agent does the wrong thing; the agent reads it in context immediately before action.
3. **Schema.** Standard memento frontmatter only (Tier 1, ADR-0014). The body is free-form prose. Per-verb soft conventions (recommended headings, ADR header shapes, etc.) may be suggested but never enforced — the file is consumed by an LLM, not a parser.
4. **Mode.** `read-only`. Tool-read files are curated conventions, not appended to. The pre-commit edit window (ADR-0017) still applies during initial drafting.
5. **Discovery.** Existence check at orient render time. No parse, no validation beyond standard frontmatter.
6. **Surfacing.** Orient's description of the paired verb includes a precondition fragment ("before `memento <verb>`, run `memento read _memento/<verb>.md`") iff the file exists. Absent → the fragment is omitted entirely; no broken pointer. The triggered-preconditions block in the orient baseline is the assembly point.
7. **Reinforcement.** None at action time. The paired verb does not check, warn, or gate on the file. Statelessness across CLI invocations is preserved (the same reason as ADR-0017's edit-window mechanism and ADR-0010's writing.md mechanism — the rejection generalises).
8. **Init posture.** Greenfield init scaffolds a minimal file alongside the verb's other init artifacts. Adoption never clobbers — if the file exists, init leaves it; if it does not, init does not create one during adoption.
9. **Filename reservation.** A tool-read filename is reserved by the ADR that lands the paired verb. `writing.md` is reserved by this ADR. `review.md` and `audit.md` are *placeholders* under ADR-0009 — they become tool-read files only when their verbs' ADRs reserve them under this pattern.

What this pattern explicitly does not cover:

- **Opinionated starter content.** This pattern says how a tool-read file is *read*; it says nothing about what it should *say*. Opinionated default content lives with `init --template=` work (ADR-0009 deferred), not here.
- **Cross-verb files.** A tool-read file consulted by two verbs (e.g., a shared style guide) is out of scope until the third verb makes a real case. One file per verb until practice argues otherwise.
- **Precedence.** Moot today (only `writing.md` is live). If two tool-read files later concern overlapping action, precedence will be added to this addendum or to a sibling ADR.

The pattern collapses ADR-0010's "writing.md mechanism" to a class with one current member. Future verb ADRs reference this section rather than re-pinning each axis.
