---
title: "Agent integration — SessionStart orient hook and write skill scaffold"
status: accepted
mode: read-only
date: 2026-06-25
tags:
  - memento
  - agents
  - orient
  - hooks
  - skills
summary: "Builds the apparatus needed to spike tool-adherence questions empirically: a SessionStart orient hook (inject-not-instruct) that completes ADR-0013's bet by guaranteeing the dynamic orient verb actually runs; orient as a lean router with brief made pull-only; and a write skill authored at _memento/skills/write.md as a ready-to-install artifact (no install, no symlink yet). Hooks and skills are strictly additive UX over the portable CLI contract (ADR-0019). Whether to install them by default, and whether to add write-side enforcement, are deferred to the A-UAT regime ([[adr-0026-agent-uat-validation-regime]]) pending evidence."
---

# ADR-0025 — Agent integration: SessionStart orient hook and write skill scaffold

## Decision

This ADR delivers the *apparatus* for encouraging agent tool use, in a form that can be toggled on or off so its effect can be measured. It deliberately does **not** decide which encouragements ship on by default — that is evidence-gated and owned by [[adr-0026-agent-uat-validation-regime]].

### Adopted now (not evidence-gated)

1. **Orient becomes a lean router; `brief` is pull-only.** The bootloader and the orient baseline stop pushing `brief` as a "run before anything else" step. Orient names the doc landscape and says *run `brief` when you need it*; `brief`'s 300-plus dense lines are a cost a short, targeted task should not pay. This is context hygiene (AGENTS.md "context-injection discipline"), independent of any measurement. This **amends convey-point 2 of ADR-0024**: the entry sequence keeps `orient` first, but `brief` moves from mandatory-second to on-demand. The other three convey-points (substrate identity, read primitive, boundary warning) are unchanged.

2. **SessionStart orient hook — inject-not-instruct.** A harness SessionStart hook runs `memento orient` and injects its output as session context, rather than relying on the bootloader text to make the agent choose to run it. This **completes ADR-0013's bet** rather than revisiting it: ADR-0013 kept the bootloader minimal precisely so the dynamic, version-locked content could live behind the orient verb; the hook is the missing piece that guarantees the verb actually fires. The output is injected directly, so the bootloader's "run orient" line becomes a conditional fallback (for harnesses where the hook did not fire / sub-agents that did not inherit). Orient is read-only and cheap, so a redundant call is harmless; injecting rather than instructing is the clean dedup. The hook re-fires on resume/compact so orient survives context loss.

3. **Write skill authored at `_memento/skills/write.md`.** A harness-native skill whose body directs the agent to `memento read _memento/writing` before composing and to write through `memento write` (not native edits). It is a *projection* of `_memento/writing.md`, which remains the single source of truth (ADR-0008 projection pattern; ADR-0010 owns the guide). **No install, no symlink** — the file exists as a ready-to-install artifact so the A-UAT regime can compare adherence with it installed vs. not.

### Invariant: additive only (ADR-0019)

Hooks and skills are **strictly additive UX**. The CLI is fully functional without them and remains the contract every subprocess-spawning agent binds to. Failing to detect an agent family, or failing to install a hook/skill, degrades discoverability — never functionality. This is what neutralises the per-harness fragmentation tension with ADR-0019: these are extra bright flashing lights, not functional requirements.

### Posture for when install lands

When installation is wired (gated below), it is **repo-local by default** (not global), baked into `init` with opt-out (`--skip-skills`) and family selection (`--agent claude`, etc.). The detect-state logic ("is the hook/skill installed for the detected agent?") is shared with the future `doctor` verb (configuration-state check, as distinct from `review`'s content check).

## Items to be validated with evidence before decisioning

These are intentionally **not decided here**. They are the questions the apparatus above exists to let us spike; [[adr-0026-agent-uat-validation-regime]] owns running them.

- **Default-install the orient hook and/or write skill?** Does the apparatus measurably lift adherence (agents reach for `orient`/`read`/`write` instead of grep/cat/native-edit) enough to justify shipping it on by default at `init`?
- **Does the write skill beat the CLI write-precondition?** The skill's only claim over an orient instruction to "read the writing guide before writing" is compaction-durable, harness-selected triggering. Adopt the skill only if evidence shows the CLI-precondition path is actually skipped.
- **Write-side enforcement.** The read-only guarantee leaks today: it holds only when writes route through `memento write`, and a native `Edit`/`Write` bypasses the mode check. Whether to add a `PreToolUse` gate redirecting native vault writes through `memento write` depends on measuring how often the leak actually occurs.
- **Read-side interception (grep/cat → `read`).** Parked. Revisit only if A-UAT surfaces a concrete miss; vault-path scoping makes it more tractable than previously assumed, but it stays lower priority than the write-side leak.

## Context

Beads solves the same priming-adherence problem with a SessionStart hook (`bd setup claude` installs exactly one: "Claude Code hooks (SessionStart)") that forces its priming verb to run, plus a per-recipe skill on harnesses like Codex. Memento's equivalent priming verb is `memento orient` (ADR-0013), but until now it relied entirely on the inert bootloader pointer (ADR-0013/0024) to get run — the same weak, compaction-prone instruction channel that motivated adding a hook in the first place.

The internal-consistency argument anchors the whole ADR: if static bootloader instructions were reliable, we would not need a hook; and if an in-context "load the writing guide before writing" instruction were reliable, we would not need a skill. The hook (always-on half) and the skill (on-demand half) are the same architectural move applied to the two halves of the tool-read pattern — moving the "remember to load this" burden off fragile conversation text and onto a harness mechanism that survives compaction.

What we resisted: deciding adoption ahead of evidence. The skill in particular has a weak a-priori case (`_memento/writing.md` is ~20 lines, so the progressive-disclosure saving is small; the only real win is trigger reliability). Rather than ship it speculatively, we build it and let A-UAT decide — hence "apparatus now, adoption later."

Alternatives considered:

- **Inject `brief` at session start (beads-style memory dump).** Rejected: brief is dense and large; a targeted fix should not pay that context tax. This is also the dump pattern memento §1 rejects. Orient (bounded by intent) is the right session-start payload; brief stays pull-only.
- **Ship hook + skill default-on now.** Rejected: that pre-empts the measurement the apparatus exists to enable, and commits harness-specific surface area before we know it earns its keep.
- **Bootloader instruction only, no hook.** Rejected: it is the status quo whose unreliability motivated this work.

## Consequences

- The repo gains a togglable orient hook and a `_memento/skills/write.md` artifact — enough to run the first A-UAT spikes.
- Orient and the bootloader stop advertising `brief` as a mandatory second step; `brief` becomes explicitly on-demand. This amends ADR-0024 convey-point 2 (see Decision); the other three convey-points are untouched.
- `_memento/skills/` is introduced as a directory. Its long-term management (Obsidian-visible, symlink direction) is deferred to [[adr-0028-obsidian-managed-skills]]; for now it is a plain authored file.
- The write skill carries skill-format frontmatter (`name`/`description`), which does not match memento's canonical frontmatter vocabulary (ADR-0014). This collision is real and is the crux of [[adr-0028-obsidian-managed-skills]]; it is tolerated here because the file is not yet compiled-as-vault-content nor installed.
- No `init` behavior changes yet: installation is gated on A-UAT. When it lands, it is repo-local with opt-out, sharing detect logic with a future `doctor` verb.

## Related

- [[adr-0013-orient-verb-and-minimal-bootloader]] — the bootloader/orient mechanism this hook completes: orient was kept dynamic *so that* a forcing mechanism like this could guarantee it runs.
- [[adr-0024-bootloader-contents]] — the four convey-points; unchanged, except `brief` shifts from push to pull.
- [[adr-0010-tool-read-writing-guide]] — `_memento/writing.md` ownership; the write skill is a projection of it, not a fork.
- [[adr-0008-memento-brief-projection]] — projection pattern: one canonical source, multiple consumer-shaped surfaces.
- [[adr-0019-descope-mcp-transport]] — the CLI-as-contract decision the additive invariant keeps faith with.
- [[adr-0026-agent-uat-validation-regime]] — owns the evidence that gates default-install, the write skill, and write-side enforcement.
