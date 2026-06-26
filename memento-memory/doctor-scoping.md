---
title: Doctor — scoping list
summary: "Live, exploratory list of candidate checks for the future `memento doctor` verb, consolidated from the ADRs and dogfooding. Doctor owns mechanical plumbing/health (no content judgement, no agent); this note is the running inbox of candidate checks so the eventual doctor ADR doesn't re-derive them. Not a spec — see [[review-audit-doctor faculties]] for the verb-boundary carve."
tags:
  - memento
  - doctor
  - roadmap
  - scoping
  - open-question
mode: living
status: proposal
date: 2026-06-26
---

# Doctor — scoping list

A running, exploratory inbox of checks `memento doctor` could perform. **Not a spec and not a commitment** — doctor has no ADR yet. The dividing principle (doctor = closed-world *machine/config health*, mechanical, no agent; vs `review` = content form/consistency; vs `audit` = open-world epistemic integrity) lives in [[review-audit-doctor faculties]]. This note just collects the concrete checks so they aren't lost between sessions.

## Plumbing / machine-state (doctor's core)

- `memento` on PATH / installed; binary version against manifest `schema_version` (warn on `manifest-schema-unsupported` risk).
- SessionStart hook installed and pointing at the current script for the detected agent family (shared detect-state logic with init — ADR-0025).
- Write skill installed for the detected agent (ADR-0025).
- Pre-commit hook present, current, and executable.
- Manifest freshness — stale vs on-disk notes.
- Config validity (`.memento/config.toml`).
- Vault discoverability (marker dir present and resolvable from repo root).
- Ignore correctness — `.mementoignore` and the repo `.gitignore` memento stanzas present and well-formed.
- Presence of expected tool-read / convention files; missing `writing.md` as a soft signal ("this vault has no project writing conventions" — ADR-0010, deferred there to doctor).
- Malformed conventions — missing/empty `when_to_read:`, disallowed frontmatter keys (ADR-0029 routes these to doctor).

## Write-enforcement liveness ([[adr-0031-remove-write-verb-hook-enforced-native-writes]])

With the `write` verb gone, mode enforcement rests entirely on the PreToolUse gate firing — and that failure is **silent** (the harness is fail-open on hook absence, crash, or a missing dependency). Doctor is the only loud surface for "is enforcement actually on", so these are a **hard dependency** of ADR-0031, not nice-to-haves:

- **PreToolUse gate installed** for the detected agent family — present in `.claude/settings.json` (and the codex equivalent), matcher includes `Write|Edit|MultiEdit|Bash`, command path resolves and is executable.
- **PostToolUse compile hook** present and path-gated to vault writes.
- **`check-write` reachable** — `memento` on PATH, binary version ≥ manifest schema (a gate that shells to a missing/old binary enforces nothing).
- **Interpreter deps present** — whatever the gate wrapper needs (e.g. `python3`); the dependency that currently fails silent.
- **Live-fire self-test** — synthesise a PreToolUse payload for a known ratified read-only note and assert `check-write` returns *deny*. The only check that proves the *chain* works, not just that parts exist.
- **No legacy broad-deny entry** — a PreToolUse hook pointing at the pre-ADR-0031 broad-deny guard would brick the vault (deny + no `write` verb to satisfy it). Flag for `--fix`.
- **Stale-grant check** — warn on `.memento/` unlock-sidecar entries with no matching uncommitted edit.
- **Headline status line** — a blunt `vault write enforcement: LIVE / OFF (reason)`. Because the failure is quiet, the status must be loud.
- **Orphan cleanup (`--fix`)** — `init` is additive-only, so doctor owns deleting the retired write-skill (`.claude/skills/memento-write/`, `_memento/skills/write.md`) and any legacy hook entries on upgrade.

Supersedes the earlier "Write skill installed for the detected agent" check above (ADR-0025) — it **inverts** to "*no* stale write-skill present".

## Discovery / onboarding (from 2026-06-26 second-cloner review)

These close the "a second person clones the repo — what is this `_memento` stuff, and where's the tool?" gap. Install info lives in memento's own README, which does **not** travel into a user's vault.

- Project `README.md` mentions memento — so a human browsing a cloned repo has a top-level discovery breadcrumb (the one surface people actually read first). Soft signal if absent.
- Vault-present-but-binary-absent: if a vault exists but `memento` isn't on PATH, surface the install path (https://github.com/tpisel/memento). This is the deliberate complement to keeping the AGENTS.md bootloader pure: per ADR-0024 we chose *not* to put an install pointer in the bootloader, so non-Claude agents (codex/Cursor, no SessionStart hook) hit `memento orient` → command-not-found with no breadcrumb. Doctor is the sanctioned place to catch that. (Claude agents already get the referral via the SessionStart hook fallback; humans via `_memento/Using Memento.md`.)

## Form checks (uncertain home)

- Per-note frontmatter/style well-formedness ("form", level 1 in the review altitudes). Mechanical, so arguably doctor-adjacent, but it could equally live in `review` or `compile`. Open question in [[review-audit-doctor faculties]]; listed here only so it isn't forgotten.

## Boundaries to keep in mind

- Mechanical only — no content judgement, no agent invocation. Anything needing taste is `review`/`audit`, not doctor.
- Doctor likely needs no tool-read file (nothing local to opine on; it checks tool-defined state).
- Output shape, exit-code semantics, and `--fix` affordances are all unscoped.
