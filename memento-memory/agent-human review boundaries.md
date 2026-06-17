---
title: Agent / human review boundaries
status: proposal
mode: append-only
date: 2026-06-13
tags: [memento, proposal, open-question, agents, beads, review, philosophy]
summary: A philosophical sketch and tentative proposal — the beads/memento substrate boundary may be the surface of a deeper axis distinguishing who *authors* and who *reviews* at each layer of work. Names a review-granularity ladder (invariants → epics → atoms) and a possible bead-epic ↔ memento-note coupling. Deliberately unresolved — recorded so future design work can return with usage evidence.
---

# Agent / human review boundaries

A note that is philosophical as much as it is processual. It records a working perspective on the architecture of human-agent collaboration — of which memento is one instantiation — without committing memento to any specific accommodation. Treat as a thinking aid for future design conversations, not a directive.

## The deeper axis

Memento currently describes the substrate split as **operational vs architectural**: beads holds task state, memento holds durable knowledge. That framing is true at one level of resolution. Underneath it, a second axis seems to matter at least as much — *who authors* and *who reads* each layer of content:

|                       | Agent-primary author | Human-primary author     |
|-----------------------|----------------------|--------------------------|
| **Agent-primary reader** | bead progress, close notes | memento ADRs, spec, future `writing.md` |
| **Human-primary reader** | bead status views, commit messages | this conversation, PR review |

Memento sits diagonally (human authors, agent reads). Beads is closer to symmetric. AGENTS.md is mostly produced as-needed by either party but blessed by the human. The substrate boundaries thus carry not just *what kind of content* but *whose attention is on the hook* for the content's accuracy.

This is the axis the spec is presently silent on, and the more it stays silent, the more our intuitions about *what the tools should do* will diverge — because intuitions about ownership pull in directions that intuitions about content type do not.

## The granularity ladder

The interesting move is to ladder the author/reader split by *review cadence*. A candidate shape:

| Layer        | Substrate                            | Cadence              | Smallest reviewable unit            |
|--------------|--------------------------------------|----------------------|-------------------------------------|
| Invariants   | memento + AGENTS.md                  | Audit / rarely       | An ADR diff, a constraint edit      |
| Epics        | beads epic + paired memento narrative | Per chunk of work    | A bead-epic close, a narrative diff |
| Atoms        | beads task + code commits            | Per PR / spot-check  | A close note, a code diff           |

The interesting claim this would make is that **the smallest reviewable unit shifts by layer**, and that *agent autonomy ought to scale inversely with layer height*. A human approves the shape of an epic but delegates atom-level QA to the agent. A human authors invariants but lets the agent surface their breaches. The ladder gives human attention a way to delegate downward without losing the ability to spot-check at any level.

Current agent tooling (Claude Code, Cursor, et al.) defaults to per-PR review and visibly breaks once an agent can land dozens of commits in a session. Per-line is wrong-grained for the same reason. Per-epic is candidate-shaped: large enough to carry meaning, small enough to bound a coherent human attention session.

## A possible bead-epic ↔ memento-note coupling

One concrete instantiation worth holding loosely:

- A **bead epic** is the agent's work-atom container.
- A **memento note** can be the human's *narrative* for that same epic — the *why*, the *scope*, the *rejected alternatives*, what *done* looks like.
- The two stay linked by stable key reference (ADR-0007 already commits to this), but each is authoritative in its own substrate.

The mechanism for this exists today. ADR-0007 explicitly pins memento key and section-anchor stability so external systems including beads can hotlink them. A bead epic with `--external-ref memento:_memento/epics/foo.md` works without any tool change. The question is not *can we?* but *should the tool name and bless the pattern?* — which is a separate, later question.

## What this is explicitly not

- **Not a reversal of spec §12's rejection of Obsidian Tasks.** Tasks-in-markdown as a task substrate creates exactly the working-memory-into-semantic-memory leak ADR-0007 sorts out. The narrower coupling described above does not require an Obsidian Tasks integration.
- **Not a proposal that memento enforce structure on how vaults are organised.** Memento remains content-agnostic (ADR-0003, ADR-0009). If a user organises by epics, fine. If they do not, also fine. The tool surfaces whatever is there.
- **Not a feature request for v0, v1, or any pinned version.** This note is upstream of any roadmap.

## Why this stays unresolved

The temptation when an interesting perspective lands is to bake it into the tool immediately. Resist:

- Spec §14's design principle is to grow structure from observed failures, not a-priori tidiness. The same principle governs roadmap decisions, not just code layout.
- We have not yet hit the failure mode this would solve. We have not run multiple bead epics with paired memento narratives. We have not observed where per-PR review breaks down for *this* project specifically.
- Naming the pattern in a tool surface before it has been lived means baking in a specific shape, and the actual shape — when it arrives — will probably differ.

So this note exists to record the thought, not act on it. Future ADRs may return to it once evidence accrues. Append below this section if the perspective evolves.

## Threads worth following

- **Meta-coherence as a review signal.** A future `memento review` (ADR-0006) could surface memento notes referenced by no bead, or beads referencing missing memento keys — symptoms that the substrate boundary is being honoured or violated.
- **A human-facing parallel to the brief.** `memento brief` is the agent-facing session-start digest. Is there a coherent *human-facing* equivalent — a digest of what the human is on the hook for this session — or is that a category error because humans use Obsidian's own native browse surface?
- **The "smallest reviewable unit" question deserves its own treatment**, separately from memento. The question is general to agent-driven development; memento is one place the answer might be encoded but not the only one.

## Provenance

Originated in a conversation on 2026-06-13 about whether the `_memento/` namespace might eventually need to demarcate not just "tool-relevant" but "agent-concern vs human-review-surface." The author noted that the spec is agnostic on this; this note records the agnosticism explicitly rather than letting it drift unspoken.

## Addendum 2026-06-17 — bead-pending-human as the first surfaceable signal

A design pass reviewing in-situ agent feedback returned to one face of the unresolved question above: when a bead is explicitly assigned to a human (epic review, API-key provisioning, design call, anything an agent cannot complete), what surfaces it to the human in a way that respects their existing reading habits?

The motivating chain is sharper than the original note framed:

1. The beads-vs-Obsidian split already demarcates an *informational-audience* boundary — beads holds machine-graph state, Obsidian is the human's reading surface. The substrate boundary is not just task-vs-knowledge; it is also CLI-shaped vs document-shaped, and humans live mostly on the document side.
2. `bead assigned to human` is a genuinely good flow at the DAG level — it represents the handoff explicitly, blocks the right dependents, and shows up in `bd ready` for whoever the next agent is.
3. But `bd list` / `bd show` is *not* a good interface for the human side of that handoff. Humans don't reach for the CLI to discover their queue; they open Obsidian.
4. Therefore: the natural surface for "what a human is blocking on" is the vault itself, projected as Obsidian-renderable content (`- [ ]` task syntax being the obvious carrier).

This directly answers the original note's *"A human-facing parallel to the brief"* thread, but tentatively and from the opposite direction: there is not a memento-shaped human digest verb; there is a *projection* of beads state into Obsidian's native browse surface, which Obsidian already renders well via its Tasks ecosystem.

### Shapes considered, none built

Four candidate shapes were weighed:

1. **Memento emits the projection** — a `memento sync-beads` verb (or compile-time integration) reads bead state and writes a `_memento/human-pending.md` (or similar) file. Cost: memento gains a hard-coded dependency on beads' assignee semantics.
2. **Beads emits the projection** — beads gains a `--vault-mirror` config that writes into the memento vault on bead state change. Cost: beads gains vault-write capability and memento-layout awareness.
3. **Convention only** — no tooling; document that assigning agents should also write a `- [ ]` line referencing the bead key, by hand. Cost: discipline-shaped, fragile.
4. **Template scaffolding** — init optionally scaffolds a `_memento/human-pending.md` stub; user wires up sync themselves (bd hook, script, cron) if they want it automated. Cost: low; punts the integration shape to whoever has the use case.

None warranted a build now. The use case is real but unforced (no missed-handoff incident has yet cost time), and per-shape coupling cost is non-trivial — locking in either direction's dependency before the shape is pressured by usage is the failure mode spec §14 warns against.

### Crystallisation: "templates as opinionated overlay" as a posture

The most useful thing this pass produced isn't a decision about beads-mirror; it's a clearer articulation of *how* memento can hold two seemingly contradictory commitments at once:

- **Core memento stays minimal and dependency-free.** Works without beads, without Obsidian, without any specific workflow opinion. This is load-bearing for adoption — ADR-0007's "integrated not merged" posture rests on it.
- **Templates scaffolded at init can be opinionated and dependent.** A template that wires up bead-mirror, a template that establishes a per-epic narrative convention, a template that pre-stamps a review-queue overlay — these are *opt-in opinionatedness* that doesn't compromise the core.

The boundary lets memento be principled (core) and opinionated (templates) without either contaminating the other. ADR-0010 already established `_memento/writing.md` as a project-curated overlay surface; the bead-mirror question is one face of a more general "what other overlay roles want to exist?" question that templates can answer without ADR-by-ADR ceremony.

This framing reshapes the bead-mirror question itself: it isn't "should memento integrate with beads?" (answer: no, that would violate core minimality), it is "should a template ship that wires up the integration, and what shape does the template take?" That's a much smaller, more deferrable question, and one that can be answered by whichever user first hits the missed-handoff incident.

### Link to the general review-queue question

Bead-pending-human is one signal of "humans should look at this." ADR-0023 (summary staleness) just introduced a second: `stale` and `missing` summaries are also "humans should look at this" signals, surfaced in brief and read stderr. A third lives nearby — agent-flagged-uncertain ("I wrote this but I'm not sure about it") — though it has no surface yet.

When 2-3 distinct signal sources exist, the general shape becomes worth attacking: a unified review-queue posture that subsumes them, possibly the home for the v4 `review` verb (ADR-0006). The bead-mirror question is one input to that future shape; building the narrow mirror now would foreclose the general answer. Wait.

### Status of the open thread

The "human-facing parallel to the brief" thread in the original *Threads worth following* section is now half-answered: yes, there is one; no, it is not a memento verb; it lives in Obsidian's native task surface, populated by a projection mechanism that may or may not eventually be ours to ship. Append further evolutions of this thread below as evidence accrues.
