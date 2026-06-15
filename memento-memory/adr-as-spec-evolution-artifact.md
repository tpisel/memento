# ADRs as spec-evolution artifacts

ADRs are worth keeping in AI-assisted development, but not because the inherited convention is sacred. They work because they are close to a useful agent-memory primitive: scoped, textual, decision-shaped, historically indexed, human-readable, diffable, and cheap to retrieve.

The useful core is not the genre of an “Architecture Decision Record”. The useful core is:

> a durable constraint on future work, with enough rationale to stop a human or agent from undoing it accidentally.

That remains valuable. But classical ADRs also carry some human-org ceremony that becomes questionable when the primary downstream reader may be a context-stuffed coding agent rather than another architect in a review meeting.

## The regime change

Pre-AI documentation mostly transferred knowledge between humans across time.

AI-era project documentation also shapes the probability distribution of future edits.

That makes artifacts less like prose explanation and more like a control surface. A good ADR should not merely tell the story of a decision. It should constrain future behaviour, raise retrieval salience for the right tasks, and make stale guidance visibly supersedable.

## What to keep from classical ADRs

Keep the parts that make ADRs unusually compatible with agentic development:

- they are small enough to retrieve and reason over;
- they are versioned alongside the code;
- they capture rationale, not just outcome;
- they are human-readable without a tool;
- they can be linked from specs, tests, and implementation files;
- they create a historical trail of why the system became shaped as it is.

The format is useful because it is boring. Boring formats survive tool churn.

## What to discard or weaken

Classical ADRs often assume that decisions are rare, discrete, mostly final, and made through human deliberation. AI-assisted development breaks that assumption.

Agents make many local design moves. They retry. They discover constraints empirically. They accidentally regress things when a later task retrieves the wrong slice of context. A record that reads like solemn committee minutes is often the wrong shape for this world.

In particular, avoid:

- long narrative “context” sections that bury the invariant;
- ritualistic “alternatives considered” sections padded after the fact;
- accepted-but-stale decisions with weak supersession semantics;
- architectural prose that is not linked to tests, files, or failure modes;
- records that describe transient debugging weather as if it were durable climate.

## ADRs as constraint packets

For agent/human co-development, an ADR should be treated as a versioned constraint packet.

The most load-bearing fields are:

1. **Scope** — where this decision applies.
2. **Decision** — what is now normative.
3. **Invariant** — what must not be violated without superseding the ADR.
4. **Rationale** — why this constraint exists.
5. **Failure mode guarded against** — what bad future edit this prevents.
6. **Operational consequences** — what code, tests, migrations, or workflows must reflect the decision.
7. **Supersession metadata** — what this replaces, conflicts with, or is replaced by.

For agents, the invariant and failure mode are often more useful than the narrative rationale. Humans infer “do not cross this line” from prose; agents benefit from being told where the line is.

## Suggested ADR template

```md
# ADR-014: Use markdown files as the canonical memory substrate

Status: accepted
Date: 2026-06-15
Scope: memory storage, retrieval, git sync
Supersedes: ADR-009
Superseded-by:
Conflicts-with:
Related: SPEC-003, TEST-memory-roundtrip

## Decision

Use human-readable markdown files as the canonical memory store. Derived indexes may exist, but they are rebuildable artifacts, not sources of truth.

## Invariant

No semantic memory may exist only in the vector index.

## Why

Agents need fuzzy retrieval, but humans need inspectable, reviewable, versioned state. Vector-only memory is too opaque and too easy to corrupt silently.

## Rejected options

- SQLite as source of truth: better structure, worse human inspection.
- Vector DB as source of truth: too opaque and hard to audit.
- JSONL only: diffable, but unpleasant as the primary human surface.

## Operational consequences

- All generated indexes must be reproducible from markdown.
- Memory writes go through normal file patches.
- CI should fail if index/schema assumptions drift.
- Retrieval code should treat the markdown substrate as canonical when conflicts arise.

## Failure mode guarded against

An agent retrieves a stale embedding and treats it as canonical truth.

## Checks

- `make test-memory-roundtrip`
- `make rebuild-index && git diff --exit-code`

## Obsolescence condition

This ADR should be revisited if a different substrate can provide human-readable review, stable diffs, cheap retrieval, and deterministic index regeneration without increasing operational opacity.
```

## Field guidance

### Status

Use statuses sparingly and mechanically.

Suggested values:

- `proposed`
- `accepted`
- `deprecated`
- `superseded`
- `rejected`

Avoid ambiguous soft statuses like `maybe`, `in discussion`, or `probably`. If uncertainty is material, put it in the rationale or obsolescence condition.

### Scope

Scope is first-class because agents overgeneralise.

Bad:

```md
Scope: storage
```

Better:

```md
Scope: semantic memory storage, generated retrieval indexes, git sync behaviour
```

The scope should tell a future agent when to retrieve the ADR and when not to apply it.

### Decision

The decision should be terse and normative.

Bad:

```md
We explored several options and markdown seemed like the best compromise.
```

Better:

```md
Markdown files are the canonical memory store. Vector indexes are derived artifacts.
```

### Invariant

This is the anti-regression spell.

The invariant should be phrased so a future implementation can be judged against it.

Bad:

```md
Keep things human-friendly.
```

Better:

```md
Every persisted memory must be inspectable and editable as plain text in the repository.
```

### Why

The rationale should explain the tradeoff, not perform sophistication.

Prefer:

```md
This prevents silent divergence between the human-readable substrate and agent retrieval state.
```

Avoid:

```md
This creates a more robust, scalable, future-proof architecture.
```

### Rejected options

Keep this section, but compress it. It is useful when a future agent is tempted to “rediscover” a rejected path.

Each rejected option should include the decisive reason, not a full miniature essay.

### Operational consequences

This is where ADRs become more than prose.

Good operational consequences point to concrete surfaces:

- tests;
- linters;
- CI checks;
- migrations;
- file paths;
- command-line behaviours;
- agent instructions;
- expected failure cases.

### Failure mode guarded against

This field is unusually valuable for AI development. It tells the agent what kind of future mistake the ADR exists to prevent.

Examples:

```md
A later agent adds memory directly to the vector store and bypasses the markdown substrate.
```

```md
A refactor treats provider-specific model settings as global defaults and breaks portability.
```

```md
A generated spec updates behaviour without updating the tests that encode the old contract.
```

### Obsolescence condition

Every accepted ADR should say what would make it obsolete.

This keeps ADRs from becoming a constitutional accretion layer: old decisions that still retrieve well, but no longer describe the living system.

## Relationship to other artifacts

ADRs should not absorb every kind of project memory.

A useful separation:

| Artifact | Purpose | Normative? | Agent use |
|---|---|---:|---|
| `spec.md` | Desired behaviour, interfaces, system model | yes | high |
| `adr/*.md` | Durable decisions, invariants, tradeoffs | yes | high |
| `notes/*.md` | Working notes, research, unresolved questions | no / mixed | selective |
| `logs/*.md` | Attempts, debugging history, transient observations | no | low |
| `tests/*` | Executable memory | yes | very high |
| `index/*` | Derived retrieval layer | no | implementation detail |

The common failure is promoting work-log material into ADRs too early. AI development produces many plausible lessons. Most are local weather. Promote only claims that should constrain future edits.

## Promotion rule

Create or update an ADR when a decision is expected to affect future implementation choices.

Do not create an ADR merely because:

- an agent tried something;
- a bug was fixed;
- a local workaround was found;
- a preference was expressed once;
- a design note sounds architectural.

A note becomes ADR-worthy when at least one of these is true:

- future agents are likely to regress it;
- the decision rejects an attractive alternative;
- the decision defines a source of truth;
- the decision constrains multiple files or subsystems;
- the decision resolves an ambiguity in the spec;
- the decision should be linked to tests or CI;
- stale versions of the decision would be dangerous.

## Retrieval guidance

Agents should not ingest all ADRs blindly. That turns the repo into a small legal system and the agent into a junior lawyer with grep.

Prefer retrieval by:

- task scope;
- explicit links from the active spec;
- file/path ownership;
- recency, only as a secondary signal;
- supersession metadata;
- tags or frontmatter where useful.

An accepted but old ADR may still be normative. A recent ADR may merely be proposed. Retrieval should privilege status, scope, and supersession over timestamp alone.

## Naming and structure

Use boring names.

```text
adr/
  001-markdown-as-canonical-memory.md
  002-derived-vector-indexes.md
  003-agent-write-paths.md
```

Each ADR should have a stable identifier. Renaming for clarity is acceptable, but avoid breaking inbound links unless the project has link validation.

## Frontmatter option

If the project benefits from machine indexing, use light frontmatter.

```yaml
---
id: ADR-014
title: Use markdown files as the canonical memory substrate
status: accepted
date: 2026-06-15
scope:
  - memory-storage
  - retrieval
  - git-sync
supersedes:
  - ADR-009
superseded_by: []
conflicts_with: []
related:
  - SPEC-003
  - TEST-memory-roundtrip
tags:
  - memory
  - substrate
  - retrieval
---
```

Do not let the metadata become heavier than the decision. The frontmatter exists to support retrieval and validation, not to recreate Jira in YAML.

## Anti-patterns

### The ceremonial ADR

Looks official, constrains nothing.

Symptoms:

- long context;
- weak decision;
- no invariant;
- no failure mode;
- no linked tests or implementation consequences.

### The fossil ADR

Once true, now stale, still retrieved by agents as if normative.

Fix with explicit supersession, deprecation, and periodic validation.

### The work-log ADR

Records what happened, not what must remain true.

Move to `notes/` or `logs/` unless it constrains future work.

### The aesthetic ADR

Says “we prefer X” without naming the failure mode or tradeoff.

Preferences are allowed, but should be marked as such. Do not disguise taste as architecture.

### The everything ADR

Combines multiple decisions into one omnibus document.

Split decisions when they have different scopes, failure modes, or obsolescence conditions.

## Suggested agent instruction

```md
When making changes, retrieve ADRs whose scope overlaps the task. Treat accepted ADRs as normative constraints unless they are explicitly superseded. If implementation pressure suggests violating an accepted ADR, do not silently proceed: either preserve the invariant, propose a superseding ADR, or mark the conflict in the task output.

When creating a new ADR, keep it short. Prioritise decision, invariant, failure mode, operational consequences, and supersession metadata over narrative context. Do not promote transient debugging notes into ADRs unless they should constrain future implementation.
```

## Summary

ADRs survive the transition to AI-assisted development because they are already close to what agents need: durable, local, textual constraints with provenance.

But the form should mutate. Less committee minute, more constraint packet. Less “we considered the following elegant alternatives”, more “do not violate this invariant unless you supersede this record”. Less fossil archive, more living spec-evolution layer.

The useful ADR is an anti-regression spell with provenance.
