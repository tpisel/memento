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
