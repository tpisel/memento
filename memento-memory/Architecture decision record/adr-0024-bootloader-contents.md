---
title: "Bootloader contents — what the AGENTS.md sentinel block must convey"
status: accepted
mode: read-only
date: 2026-06-17
tags:
  - memento
  - bootloader
  - agents
  - orient
  - conventions
summary: "ADR-0013 pinned the bootloader *mechanism* — a sentinel-bounded block in AGENTS.md/CLAUDE.md, written at init, otherwise inert. This ADR pins the bootloader's *contents*: the four convey-points every realisation of the block must carry — substrate identity, entry sequence (orient before brief), read primitive (read for keys/sections, not grep/cat), and the discoveries-outlive-task-into-memento boundary warning. Exact wording is implementation; the four convey-points are the contract."
---

# ADR-0024 — Bootloader contents

## Decision

The AGENTS.md / CLAUDE.md sentinel-bounded block ("the bootloader"), established by ADR-0013 as the *mechanism*, has its *contents* pinned here. Every realisation of the block — current and future — must carry the following four convey-points. Exact wording is implementation detail; the convey-points are the contract.

### Convey-point 1: substrate identity

The block must name what kind of content lives in the memento vault and what does not. Two halves:

- **Positive**: durable project knowledge — design decisions, specs, constraints, discoveries.
- **Negative**: not task state, not transient work-in-progress, not the agent's working memory.

This disambiguates memento from the project's other memory substrate(s) — beads most often, but the boundary holds for any task store the project uses. Without the negative half, agents conflate substrates and the durable layer rots into a working-memory swamp (spec §1).

### Convey-point 2: entry sequence

The block must direct the agent to `memento orient` first, then `memento brief`, before any other memento action. The order matters:

- **`orient` first** because it establishes the verb contract, write modes, and triggered preconditions (ADR-0013). Without orient, brief is interpretable but the agent does not know what to do with what it finds.
- **`brief` second** because it is the entry index — the scannable surface for tag/title/summary-based filtering before any `read`.

Anything before this sequence is wrong-shaped: a `grep` of the vault skips the agent contract; a direct `read` without the brief skips the filter step.

### Convey-point 3: read primitive

The block must direct the agent to `memento read` for retrieval, with two halves:

- **Positive**: `read <key | @N | key#heading>` is the retrieval primitive; it carries binding state, staleness state, and link surface on stderr (ADR-0021, ADR-0023).
- **Negative**: not `grep`, not `cat`, not file-system tools. These bypass the metadata channel and treat the vault as plain text rather than as an indexed knowledge surface.

The convey-point is what makes the manifest investment pay off in agent behavior — without it, agents fall back on shell habits and the manifest becomes overhead they do not consume.

### Convey-point 4: boundary warning

The block must state that durable learnings that outlive a task exit the task store (beads, or whatever the project uses) and enter the memento vault. This is the boundary leak warning spec §8 names as load-bearing:

> *agents will try to encode durable learnings into beads close-notes, where compaction destroys them — the bootloader and writing guide must state that discoveries outliving a task exit beads into the memento vault.*

The warning is **not optional**. Without it, agents see beads as the path of least resistance for any prose discovery and the durable layer never gets written to. Where projects use a task store other than beads, the warning re-targets to that store but does not disappear.

### What the block must not contain

Several things look tempting to add to the bootloader but belong elsewhere:

- **Verb signatures, flags, output shapes** — belong in `memento orient`. The block points at orient; it does not duplicate it.
- **Mode semantics** (`append-only` / `living` / `read-only`) — belong in `memento orient`.
- **Brief render contract details, `@N` mechanics beyond the entry-sequence pointer** — belong in `memento orient`.
- **Per-verb preconditions** (e.g., "before write, read writing.md") — belong in the orient triggered-preconditions block (ADR-0010 pattern).
- **Project-specific orientation** — belongs in `orient: true`-tagged docs or in the project's own AGENTS.md prose outside the sentinel block.

The block is the *signpost layer*. Everything past the signpost is in orient. This is what keeps it small and stable (ADR-0013) without going so small it loses load-bearing content.

### Realisation and tuning

The current realisation lives in `internal/setup/init.go`'s `bootloaderBlock()`. Tuning the wording — making it terser, adapting it to a project where the task store is not beads, rewording the read primitive — is legal and does not require an ADR. *Removing* a convey-point requires an ADR.

The four convey-points are minimum coverage, not exact text.

## Context

The bootloader is the load-bearing member of the load chain. Spec §1 names it explicitly:

> *If `AGENTS.md` does nothing else well it must do this — it is the load-bearing member, and the only thing that degrades everything downstream at once if it bloats or goes stale.*

ADR-0013 carved out the bootloader's *mechanism* — sentinel-bounded, init-managed, inert after init — and shrank its *role* from carrier (held conventions) to signpost (points at orient and brief). What ADR-0013 did not pin was *what the signpost says*. The actual wording lived in code, with no design commitment naming what it must convey. Three pressures pushed against leaving this implicit:

1. **The bootloader is the only memento-managed text outside the vault.** Every other memento surface (manifest, brief, orient, notes) lives inside the vault. The bootloader sits in `AGENTS.md` / `CLAUDE.md`, files multi-tenanted with project, beads, and other tooling. This makes its prose the highest-leverage memento copy in the entire system. Prose that high-leverage should be a stable contract, not an implementation detail of `init.go`.

2. **Future verbs will pressure the block.** Each new verb (review, audit, future surface) will tempt expansion — "add one line about review here." Without a pinned contract, the block ratchets up and becomes the swamp ADR-0013 carved it out of. Pinning the four convey-points makes the answer to "should this line go in the bootloader?" trivially the same: *no, it goes in orient*, unless it changes one of the four points.

3. **The boundary leak warning (spec §8) was prose-only.** A load-bearing instruction with no ADR backing is fragile — a future refactor of the init wording could quietly drop it. Lifting it into the convey-point set protects it.

Two alternatives were considered:

- **Pin only the mechanism; let realisations drift.** Rejected. This is the status quo and re-introduces the swamp risk above. The block is too load-bearing to leave its contents to code commits.
- **Pin the exact wording.** Rejected as over-finessing. Wording wants to adapt to projects (beads vs another task store), to agents (Claude Code vs codex idioms), and to terseness experiments. Pinning shape (the four convey-points) lets text evolve while protecting load.

The convey-point shape mirrors ADR-0014's three-tier frontmatter approach: pin the structure, leave room for tuning within it.

## Consequences

- The init wording in `internal/setup/init.go` is a *realisation* of the convey-points; it is mutable but every change must preserve all four.
- New verbs land in orient by default. Adding to the bootloader requires arguing that the addition changes one of the four convey-points — most additions will not.
- The boundary leak warning is now contract, not optional prose. Removing it requires a new ADR.
- Projects using a task store other than beads can re-target the boundary warning ("durable learnings exit Linear / Shortcut / Jira into the memento vault") without violating the contract — the convey-point is *substrate boundary*, not *beads specifically*.
- Multi-agent-family tuning (Claude Code vs Cursor vs codex idioms) is legal: the wording adapts to the family's reading habits while the four points stay.
- The block stays small. Adding a convey-point requires an ADR amending this one; absent that, the block does not accrete.
- Auditing whether a project's bootloader is healthy reduces to checking the four convey-points are present, not to comparing wording.

## Open questions

- **Agent-family-specific wording.** Some agent families respond better to imperative one-liners, others to short paragraphs. The realisation today is paragraph-shaped. Whether `init` should support a `--agent=claude-code|cursor|codex` flag to tune wording per family is deferred until multi-family adoption pressure appears.
- **Memento version pinning.** Whether the block should advertise the memento binary version is open. Lean no: orient already carries the version-locked semantics; brief is self-describing; adding version to the bootloader couples its content to a release that re-running init refreshes. Defer.
- **Tool-read file presence in the bootloader.** Should the block advertise that `_memento/writing.md` (or future `review.md` / `audit.md`) exist and shape relevant verbs? Lean no: this is orient's job per ADR-0010's tool-read file pattern (Addendum 2026-06-17). The bootloader pointing at orient is already sufficient.
- **Multi-vault bootloader contents in a monorepo.** Multi-vault is out of scope (spec §15). When it returns, this ADR may need a clause for how a bootloader pointing at multiple vaults composes the convey-points.

## Related

- [[adr-0013-orient-verb-and-minimal-bootloader]] — the bootloader *mechanism*. This ADR is its contents complement: mechanism + contents = the full bootloader contract.
- [[adr-0010-tool-read-writing-guide]] — orient-precondition surfacing for per-verb conventions. The bootloader points at orient; orient carries the per-verb precondition fragments; convey-point 3 names the read primitive that those preconditions assume.
- [[adr-0021-read-time-link-surface]] — convey-point 3 mentions the stderr metadata channel; ADR-0021 is its detailed contract.
- [[adr-0023-summary-staleness-in-ledger]] — convey-point 3 also benefits from the staleness state on stderr; ADR-0023 is its contract.
- Spec §1, §8, §11 — names the bootloader as load-bearing, names the boundary leak warning as required, names the sentinel-block mechanism. This ADR pins the contents §1 and §8 implied.
