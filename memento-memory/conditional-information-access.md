---
title: The two axes of conditional information access
summary: Conditional access decomposes into two typing axes — constative/performative (what the information is) and epistemic/deontic (how access is governed) — laid over the spec's access-pattern substrates. An exploration of what follows, not a feature plan.
tags: [theory, information-access, design-philosophy]
mode: living
---

# The two axes of conditional information access

memento's core gesture is conditional access: an agent re-internalises only the slice of an externalised memory that its situation warrants. The spec already names the axis an agent *experiences* — **access pattern** (unconditional push, scheduled pull, conditional agent-initiated pull). That axis is about routing: where content lives and how it arrives. It does not, on its own, say anything about *what is being conditioned* or *on what grounds*.

This note adds the two axes that do. They are typing axes, orthogonal to routing, and together they form a 2×2 that explains why some conditional access is cheap and recoverable while some is dangerous and only-as-real-as-its-enforcement. The claim is ontological, not procedural: this is the shape of the problem, independent of which verbs memento ships.

## The substrate axis is routing, not typing

Push / scheduled-pull / conditional-pull (AGENTS.md / beads / memento) answers *where does this content live and how does it reach the agent*. It is primary in the spec for a good reason — most of the "what should this reader see" question is answered by routing, before any per-request gate runs. Routing is determined largely by typing (below): catastrophic, behaviour-binding content is pushed unconditionally; durable, descriptive content is pulled conditionally; transient task state is scheduled. So routing is downstream of the two typing axes, which is why naming them is worth the trouble — a misrouting is visible only once you can name the type that was misrouted.

## Two typing axes

### Constative vs performative — what the information *is*

After Austin: a **constative** utterance describes a state of affairs and is truth-apt ("we rejected SQLite because the target is read-only"). A **performative** utterance enacts something — it directs, binds, or licenses ("always run `just check` before commit"; "this spec is frozen"). The operative question differs by type. For constative content the question is *relevance*: is this true thing useful here. For performative content the question is *applicability and authority*: does this directive bind this reader, and who had standing to issue it.

memento's content is overwhelmingly constative by design — "code is prime memory" restricts the vault to the *why*, the rejected alternatives, the un-AST-able residue. The performative material is mostly routed elsewhere (hard rules to AGENTS.md) or handled as the spec's **operational** file-role (`writing.md`, `orient` overlays). The spec's *content* / *operational* split is, near enough, this axis reached pragmatically.

### Epistemic vs deontic — how access is *governed*

Independently of what the content is, the *condition* on access is keyed on one of two modalities. An **epistemic** condition is about knowledge: show what is relevant-to-know, filter for usefulness. A **deontic** condition is about permission and obligation: show what this reader is *allowed* or *required* to see or do. Relevance-filtering is epistemic; clearance is deontic.

### Why they are independent

The two axes correlate but do not collapse. Constative content is *usually* governed epistemically (you withhold a fact for irrelevance, not prohibition) and performative content *usually* deontically (a rule binds or it doesn't). But each can take the other governance: a constative fact can be deontically restricted (a true thing you are not cleared to know), and a performative rule can be epistemically filtered (a directive simply irrelevant to the current task, not forbidden). Because the governance is free to vary against the content type, this is a genuine 2×2, not one distinction wearing two names.

## The four quadrants

- **Constative × epistemic** — ordinary memory retrieval. "Here is a relevant fact." The bulk of memento; the condition is relevance, and the index/body split is its native form.
- **Constative × deontic** — restricted fact. A true thing withheld by permission rather than relevance. Rare in memento by design, and the quadrant to fear: deontic conditioning of *facts* is where conditional access decays into induced ignorance (see consequences).
- **Performative × epistemic** — applicable-but-filtered instruction. Rules relevant to the task surfaced, irrelevant ones held back for economy. `orient` overlays and the write-time surfacing of `writing.md` sit here.
- **Performative × deontic** — authority-bound instruction. "This binds you / you may not change this." The AGENTS.md hard rules; and, mechanically, memento's own write-mode + ratification system, which enforces mutation rights on its notes rather than merely asserting them.

## Natural consequences

### Conditioning has a direction of safety

Epistemic conditioning degrades gracefully: a reader under-informed about something it could have known can recover, *provided it knows the thing exists*. Deontic conditioning degrades dangerously: the reader cannot distinguish "withheld" from "absent", and if anyone relied on the condition for safety without mechanical backing, the failure is silent. The bias that follows is to condition epistemically wherever possible and to treat every deontic condition as a debt that must be paid in enforcement or explicitly marked advisory.

### Existence must outrank content

The load-bearing safety mechanism is the separation of *advertising existence* from *disclosing content*: a generous, role-agnostic index (titles, summaries, headings) over conditionally-pulled bodies. This is the spec's "unknown-unknowns into known-unknowns" stated as a general law. Conditional access *without* a generous existence-layer is not retrieval; it is amnesia administered on purpose. The corollary binds any future conditioning, however sophisticated: condition the body, never the awareness.

### Performative content carries an addressee; constative does not

A fact is true regardless of who reads it. An instruction is always *for someone*. Performative content therefore has an implicit second argument — its audience — that constative content lacks. This is why an axis like "what kind of agent" arises *naturally* for performative content and feels forced when imposed on constative content: you are supplying the addressee the directive always implied. It is also why the characteristic pathology is the unaddressed imperative — a "DO NOT" with a silent universal subject, which misfires on every reader it was not written for.

### Enforcement co-locates with whoever owns the boundary

A deontic condition is real only to the extent that the layer expressing it controls the relevant affordance. memento can enforce *mutation* of its notes because it owns the write path — a ratified read-only note is physically unwritable through the tool, not merely warned against. memento cannot enforce *who may read what*, because it owns neither agent identity nor context assembly; that belongs to the harness. A deontic condition expressed in a layer that does not own the boundary is decorative. The discipline is to know, for each boundary, which layer owns it, and to express the condition only there.

### Read and write are deontic duals

Conditional *exposure* and conditional *authorship* are two faces of one relation. If performative content is addressed by role on the way out, it must be authority-constrained on the way in, or one agent authors the rules another obeys — privilege escalation by note. memento's write-modes are the write-side deontic, already mechanical; any read-side deontic would need its own enforcement *and* its own guard on who may author the content being gated. Treat the two directions as a pair or the asymmetry becomes an attack surface.

### Conditional access concentrates trust in the projector

The amnesiac cannot audit what it was never handed. Every condition is an act of trust in the conditioning layer, and the conditioner therefore holds near-total epistemic power over the conditioned. The integrity and inspectability of the projection matters more than any single note: the human must retain the god's-eye view the agent structurally lacks — every projection dumpable and reviewable — precisely because the agent cannot hold its own memory. This is the design's namesake risk: an externalised memory is also a place where someone can rewrite your past without your knowing.

## Where this leaves role and task

The axes predict two further conditioning dimensions as latent, not as commitments. "What kind of agent" is the addressee that performative content already implies; "what kind of task" is a teleological relevance prior over constative content. Both are real, and both are deferrable. Two locating facts keep them honest. First, by the spec's own governing principle — structure earns itself from observed failure — neither should be built before a named failure demands it. Second, neither belongs in the substrate: task is exiled to beads by design, and agent identity is the harness's to assert and the substrate's only to receive. Role and task are therefore *retrieval-policy priors the caller applies*, expressed against a substrate that stays a pure, generous, derived index. Building them into the core would re-import the very things adjacent layers exist to hold, and would put a deontic claim in a layer that cannot enforce it.

## Caveats on the vocabulary

Both distinctions are heuristic, not metaphysical. Austin himself dissolved the constative/performative line once he noticed that asserting is also an act — every utterance has descriptive *and* enactive force. The axis here tracks the *dominant* force for access purposes, and real notes are mixed: an ADR is constative about what was decided and performative in binding what follows. Likewise epistemic and deontic are clean as modal logics but blur at the edges of any implementation. The value is not taxonomic purity; it is that naming the dominant type of a piece of content, and the modality of the condition placed on it, predicts how that conditional access will fail — and failure mode, not tidiness, is the only thing worth designing against.
