---
title: Descope MCP transport — the CLI is the durable agent surface
status: accepted
mode: read-only
date: 2026-06-14
tags:
  - memento
  - mcp
  - cli
  - scope
summary: "MCP is removed from the roadmap. spec §13 v3 row is withdrawn. The CLI is the single durable surface agents interact with. The 'tool-layer write enforcement' property the spec credited to MCP is delivered identically by `memento write` going through the mode check — the guarantee belongs to the verb, not the transport. Beads serves as the working counter-example of a CLI-only agent workflow."
---

# ADR-0019 — Descope MCP transport; CLI is the durable agent surface

## Decision

Memento will not ship an MCP server. spec §13's v3 row (`memento serve` + MCP) is withdrawn. The CLI is the contract agents bind to.

The other v3 items (link surfaces on `read`, agent-driven summarisation) are *not* descoped — they are simply no longer blocked on or shaped by an MCP transport. Re-homing them across the version plan is a follow-on action, not part of this decision.

## Context

spec §13 scheduled an MCP server in v3 alongside link-graph navigation and agent-driven summarisation. Two threads supported the MCP commitment:

1. **Tool-layer write enforcement** — spec §8: "an agent can rationalise past a written rule, but a `read-only` doc is *physically* unwritable through the write tool." This was credited to "the MCP gate."
2. **Promotability** — spec §14: "Transports are dumb; the core is shared. The MCP is a second mouth on the same library."

Reviewing both in the cold light of dogfooding:

- The enforcement property does not depend on the MCP transport. `memento write` runs the same `WriteMode` check whether invoked via `exec` from an agent shell or via an MCP `tools/call`. A `read-only` doc is just as unwritable through the CLI write verb as it would be through any future MCP wrapper. The guarantee belongs to the verb, not the transport.
- Beads (the project's task system, dogfooded daily across multiple agent shells) demonstrates that a rich agent-facing workflow can be served by CLI alone, with no MCP. The "second mouth" was a hedge against an outcome that has not arrived and is not in evidence.

Per-agent reach also favours the CLI. Every agent shell that can spawn a subprocess can drive memento; the subset that have implemented MCP is strictly smaller and shifts on someone else's timeline.

## Rationale and consequences

What we accept by descoping:

- **MCP-native niceties** — typed tool schemas surfaced to introspecting agents, resource subscriptions, cleaner permission-prompt UX in MCP-aware agents. Real but not decisive given the beads precedent and the per-agent-reach argument.
- **Single-binary distribution simplifies.** No long-running server process; every invocation is a one-shot CLI run.
- **One surface to maintain.** The library boundary is still the contract — if a specific agent platform later requires MCP-native integration, a thin shim over the existing library can be added without rewriting the core.

Cleanups landing from this decision (tracked under epic [[memento-fb0]]):

- `memento serve` stub is removed from the CLI surface and help text.
- spec §13: the v3 row is rewritten to drop MCP; the link-surfaces and agent-summarisation items are re-homed across the remaining versions.
- spec §8 / §14: the "MCP gate" and "second mouth" framings are corrected — the enforcement property is credited to the write verb; the transport-plurality language is removed or marked withdrawn.
- Other in-vault references to MCP are audited and updated.

## Out of scope

- Future CLI shape (output formats, additional verbs, agent-facing affordances). The CLI continues to evolve under v1 polish; what it should look like as the durable surface is a separate conversation.
- Re-sequencing of the v3/v4 items that are not MCP.

## Related

- spec §8 (Write modes), §13 (Version / scope plan), §14 (Design principles — "transports are dumb").
- [[adr-0007-beads-integration-posture]] — beads as the working CLI-agent precedent referenced above.
