---
title: "Role-based autoload (thought-bubble) — per-role orientation stances"
status: draft
mode: living
date: 2026-06-25
tags:
  - memento
  - agents
  - orient
  - roles
  - thought-bubble
summary: "Placeholder. Today an agent gets one autoload stance (the orient overlay via orient: true). This thought-bubble captures the natural extension to role-based stances — worker / planner / reviewer — where each role autoloads a different orientation set. A capability to add, not a requirement for typical use. Not yet scoped; do not build."
---

# ADR-0027 — Role-based autoload (thought-bubble)

> Status: draft / thought-bubble. Captured so the idea is not lost and so [[adr-0025-agent-integration-orient-hook-and-write-skill]] has a forward link for it. Not scoped, not scheduled, not a commitment.

## The thought

Memento's current orientation model is single-stance: one bootloader, one orient baseline, one user overlay (docs tagged `orient: true`). Every agent invocation gets the same thing.

Different agent roles want different orientation:

- a **worker** wants the verb contract and the write guide;
- a **planner** wants the spec, the ADR landscape, and `brief` density;
- a **reviewer** wants the writing/review conventions and the changed-surface context.

The natural capability is **role-based autoload**: define roles and give each a distinct stance on what gets injected/registered (which orient overlay, which skills, whether `brief` is pushed). The hook (ADR-0025) and skills are the units a role would select over — this is why the apparatus shape matters for forward-compat.

## Why deferred

- It is additive and has no v1 dependency.
- It should stay **optional** — typical single-role use must not have to know roles exist.
- It is a genuinely distinct conceptual domain from "make a single agent orient reliably" (ADR-0025), so it earns its own ADR rather than bloating that one.

## Open, when picked up

- How a role is declared (flag, env, harness signal?).
- Whether roles are a memento concept or a thin mapping over the existing `orient: true` / skill selection.
- Interaction with A-UAT: each role would want its own expectation set.

## Related

- [[adr-0025-agent-integration-orient-hook-and-write-skill]] — the hook + skill apparatus a role would select over.
- [[adr-0013-orient-verb-and-minimal-bootloader]] — the single-stance overlay model this would generalise.
