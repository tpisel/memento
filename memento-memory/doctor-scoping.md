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

**Doctor itself is deferred (decided 2026-06-26).** ADR-0031 names doctor a hard dependency, but its implementation is *descoped* from the ADR-0031 build — doctor gets its own ADR later, and these stay candidate checks until then. Three consequences to carry forward:

- **No liveness signal in the interim.** With no doctor, nothing loudly reports `enforcement: LIVE/OFF`. This is a consciously accepted gap; the only backstops are the PostToolUse **drift alarm** and the **`check-write` decision log** (detective, not preventive). The future doctor ADR closes it.
- **The "interpreter deps present" check is contingent on the wrapper.** It was inherited from the old broad-deny `pre-write-vault-guard.sh`, which used python helpers. If the ADR-0031 wrapper is the pure-Go dumb pipe (`cat | memento check-write`), there is **no `python3` in the runtime hook chain** — this check should target whatever the final wrapper actually needs (bash + the `memento` binary), not python3. (`python3` survives only in the manual A-UAT scorer, which is not on the enforcement path.)
- **Orphan cleanup needs an interim owner.** `doctor --fix` was to delete the retired write-skill + legacy hook entries. Until doctor exists, a standalone migration bead removes those artifacts in-repo; cross-vault upgrade cleanup waits for doctor.

## Cadence and verb-boundary — "doctor" is being asked to span three cadences (2026-06-28)

The checks accreted onto doctor above do not share a cadence or an audience, and
the single-verb framing blurs that. Pulling them apart before doctor gets its ADR:

- **Liveness** (PreToolUse gate installed in *this* `.claude/settings.json` / codex
  config, binary on *this* PATH, live-fire self-test) is a **per-machine, per-clone**
  property. A CI/CD pre-ship call **cannot** assert it: CI runs in a different
  environment than the developer/agent machine where the hooks must fire, so "the
  hook is installed" in CI says nothing about whether enforcement is live where the
  agent writes. Liveness also wants to be **unmissable**, and a verb you must
  remember to run is the opposite — its natural home is **ambient / SessionStart**
  (the orient SessionStart hook already runs on Claude and could emit a loud
  `enforcement: OFF`), with an on-demand deep self-test as the manual escalation.
  This is *not* a "doctor as CI gate" responsibility.
- **Ratification audit** (does committed / about-to-commit content honour modes) is
  **per-commit**, and is **already homed on `compile`** (PostToolUse + the git
  pre-commit hook, with `MEMENTO_STRICT_COMMIT` as the opt-in hard gate). It was
  never doctor's job and should not migrate to one. This *is* CI-able.
- **Static hygiene** (config validity, manifest freshness, orphaned write-skill /
  legacy hook artifacts, malformed conventions) is **occasional** and CI-able. This
  is the genuine doctor — closed-world machine/config health, mechanical, no agent.

So: `doctor` is the right name for the static-hygiene bucket and the **wrong** name
for liveness (ambient/SessionStart, not CI) and for the ratification audit (already
on `compile`). The urgent-and-missing piece — the liveness signal — probably should
**not wait on a doctor ADR**: it's a SessionStart concern, and decoupling it lets
the signal ship without designing the whole health verb. See
[[review-audit-doctor faculties]] for the review/audit/doctor carve this refines.

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

## v1 `memento doctor` — liveness shipped (2026-06-29, memento-aan)

Liveness-only doctor verb landed (`internal/cli/doctor.go`). Headline `vault write
enforcement: LIVE/OFF (reason)`, exit 0 = LIVE / 1 = OFF, plus per-check lines.
Decisions settled while building, so the doctor ADR need not re-derive them:

- **Live-fire fixture = always a throwaway non-git temp vault, never the real
  vault.** The bead left "synthesise a read-only note to fire against" open. Settled:
  the probe `os.MkdirTemp`s a vault, writes a `mode: read-only` note, and runs the
  *real* verdict chokepoint (`computeVaultWriteVerdict`) against a Write that rewrites
  it, asserting a `read_only` deny. A non-git tree treats every note as ratified (the
  edit window only opens *inside* a git work tree — `note.IsRatified`), so the probe
  needs **no commit**. **Rejected: firing against a real ratified read-only ADR.** It
  would leave residue — `computeVaultWriteVerdict` calls `recordDecision`, appending a
  synthetic deny to the gitignored `.memento/decision-log.jsonl` the A-UAT scorer
  reads. The temp vault absorbs that write and is `RemoveAll`'d. This also makes the
  "no read-only note exists" case moot — the probe is self-contained. The probe proves
  the verdict *engine* of THIS binary denies; the bash wrapper + binary resolution are
  covered by the gate (#1) and binary-reachable (#3) checks, so the composite covers
  the chain.
- **A family counts only if memento actually wired it** — keyed on the installed
  `.<agent>/memento-pre-write-vault-guard.sh` script existing, NOT on the agent config
  dir existing. A bare `.codex/` from another tool (this repo's is a **beads** install:
  `.codex/config.toml` = `[features] hooks = true` + a `.codex/hooks.json` of `bd
  codex-hook …` entries, zero memento) must not force a phantom codex gate failure that
  reports OFF when Claude enforcement is live. The gate check still parses the config
  and fails loudly if the script is present but unreferenced/non-executable.
- **Check severities:** PreToolUse gate, binary-reachable (incl. schema), legacy
  broad-deny, and live-fire deny are `FAIL` → flip OFF. PostToolUse compile-hook
  missing and stale unlock grants are `WARN` only — the *preventive* gate is still live
  even if the *detective* drift backstop or grant hygiene is degraded.
- **Legacy broad-deny discriminator:** the pre-ADR-0031 guard and the current gate
  share the filename `pre-write-vault-guard.sh`, so detection is by **content**: the
  ADR-0031 dumb pipe always shells to `check-write`; a script that lacks `check-write`
  but contains `memento` + `permission_decision` is the legacy broad-deny guard.
- **Binary reachability** resolves `${MEMENTO_BIN:-memento}` via `exec.LookPath` (what
  the hook itself shells to) and compares the on-disk manifest `schema_version` against
  the compiled-in `manifest.CurrentSchemaVersion` (read raw, not via `manifest.Load`,
  which rejects unsupported versions before we can report them).
- **Windows:** no execute bit exists, so `hookExecutable` treats existence as runnable
  there (keeps CI green); the live-fire probe is pure Go + git, no bash.
- Confirms the cadence note's call: liveness shipped **without** waiting on the doctor
  ADR. Static-hygiene checks above stay deferred. Next: wire this into the SessionStart
  orient hook (memento-mbd) so the signal is ambient, not a verb you must remember.

## Liveness made ambient — SessionStart orient hook (2026-06-29, memento-mbd)

The liveness signal is now **ambient**: the SessionStart orient hook runs `memento
doctor` after compile + orient and folds the result into the injected
`additionalContext`, so `enforcement: OFF` is unmissable without remembering to run
the verb. Settles the cadence note's call that liveness wants a SessionStart home,
not a CI gate. Decisions carried forward:

- **Output folding honours the context-injection discipline.** LIVE collapses to the
  one-line headline (`${doctor_output%%$'\n'*}`); the per-check OK lines are dropped as
  noise. OFF keeps doctor's **full** report (headline + the failing check) so the break
  is actionable in place. The full first line is kept verbatim, so codex's LIVE caveat
  (`LIVE (apply_patch only; …)`) rides through intact.
- **Codex is wired, not carved out.** The Claude and codex orient scripts share one
  generator (`claudeOrientHookScriptContents`), so codex gets the same doctor line for
  free, and `doctor` itself self-reports the apply_patch-only caveat when it detects a
  codex gate (memento-aan) — no codex-specific hook logic needed. (This repo's `.codex/`
  is beads-only, so nothing is installed here, but a memento-init'd codex vault gets it.)
- **A binary too old to know the `doctor` verb is its own uncertainty signal.** The
  fold matches on the `vault write enforcement:` headline marker; a doctor invocation
  that errors without producing that marker (old binary, missing verb) is folded into a
  clear `memento doctor unavailable; cannot confirm write enforcement is live (upgrade
  memento)` line rather than injecting the raw `unknown command "doctor"` CLI error.
  Found by dogfooding: the `memento` on PATH in this repo predated the verb.
- **Three surfaces stay in lockstep:** the init generator (`internal/setup/init.go`),
  the installed artifact (`.claude/memento-orient-session-start.sh`), and the reference
  template (`scripts/agent-hooks/orient-session-start.sh`, which keeps the `go run`
  fallback). The orient script is **not** drift-pinned to the template (only the
  pre/post-write guards are), so the three are edited together by hand.
