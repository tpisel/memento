---
title: "Agent user-ability testing (A-UAT) — validating tool-encouragement levers"
status: proposed
mode: read-only
date: 2026-06-25
tags:
  - memento
  - agents
  - testing
  - a-uat
  - orient
  - hooks
  - skills
summary: "Proposes a lightweight, pre-registered behavioural test regime for deciding which agent tool-encouragement levers (orient hook, write skill, write-side enforcement) memento should ship. Expectations are fixed in advance ('prompt Z creates circumstance X; under X the agent should do Y'), a Claude subagent and/or headless Codex agent is run on prompt Z, and observed tool use is compared to expectation across a small n. Depends on the apparatus built in [[adr-0025-agent-integration-orient-hook-and-write-skill]]. The full scenario matrix and harness are out of scope here; this ADR fixes the shape."
---

# ADR-0026 — Agent user-ability testing (A-UAT)

> Status: proposed. This ADR pins the *shape* of the regime. The concrete scenario matrix, scoring rubric, and harness wiring are filled in when the regime is first run; expect this document to be restructured then around observed behaviour and per-lever performance.

## Why this is its own ADR

[[adr-0025-agent-integration-orient-hook-and-write-skill]] deliberately builds the encouragement apparatus (orient hook, write skill) without deciding what ships on by default. Those decisions need evidence about real agent behaviour, and that evidence needs a method that does not get reinvented per question. A-UAT is that method. It is the gate standing between "we built a lever" and "we ship the lever on by default."

## Decision (shape)

Adopt a pre-registered behavioural test regime:

1. **Register expectations in advance.** For each lever, write the expectation as a pair: *"prompt Z will create circumstance X"* and *"under circumstance X, a well-behaved agent should do Y."* Registering before running avoids post-hoc rationalisation of whatever the agent happened to do.
2. **Run a real agent on prompt Z.** A Claude subagent and/or a headless Codex agent, in a checkout with the lever toggled on, and a matched run with it off (the apparatus from ADR-0025 is togglable precisely for this A/B).
3. **Observe, do not trust self-report alone.** Determine actual tool use by log review (preferred) and/or post-hoc interrogation of the agent.
4. **Compare to expectation across a small n** (≈3 per scenario). This is a signal-generating regime, not a statistical one; n is kept low deliberately and the regime stays light.

## Levers under test (initial)

These are the open items handed over by ADR-0025:

- **Orient hook** — does SessionStart injection of orient measurably change whether agents orient before acting, vs the bootloader pointer alone?
- **Write skill** — installed vs not: does it lift "read the writing guide before composing" and "write through `memento write`" over the CLI write-precondition alone?
- **Write-side enforcement** — measure the read-only leak first: how often does an agent edit a vault file natively (bypassing the mode check) rather than via `memento write`? Magnitude decides whether a `PreToolUse` redirect gate is worth building.

## Candidate scenarios (illustrative, to be finalised on first run)

1. **Targeted fix** — prompt that needs no vault survey. Expectation: agent does *not* load `brief` (justifies brief staying pull-only).
2. **Write task** — prompt that should produce a durable note. Expectation: agent loads the writing guide and writes via `memento write` (justifies the write skill / precondition).
3. **Edit a read-only note** — prompt that tempts a native edit. Expectation: the write is blocked or routed through `memento write`, not silently applied natively (justifies — or kills — write-side enforcement).

## Out of scope

- Building a full A-UAT framework (fixtures, scoring automation, CI integration). That is a separate project; this ADR keeps the regime deliberately manual and light until it has earned investment.
- Any decision about the levers themselves — those land as amendments to ADR-0025 (or follow-on ADRs) once A-UAT produces evidence.

## Related

- [[adr-0025-agent-integration-orient-hook-and-write-skill]] — builds the togglable apparatus this regime measures; lists the exact items gated on these results.
- [[adr-0006-review-verb-and-agent-assisted-maintenance]] — agent-assisted maintenance precedent; A-UAT is the behavioural-evaluation analogue for the integration surface.
