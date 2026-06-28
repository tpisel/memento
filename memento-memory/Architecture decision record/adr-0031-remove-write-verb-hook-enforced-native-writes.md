---
title: "Remove the write verb — native writes under hook-based mode enforcement"
status: accepted
mode: read-only
date: 2026-06-26
tags:
  - memento
  - write
  - mode
  - hooks
  - agents
  - enforcement
summary: "Removes the memento write verb entirely. Agents write note bodies with their native tools (Write/Edit/MultiEdit/Bash on Claude; apply_patch/shell on codex); a PreToolUse check-write hook gives the allow/deny verdict pre-mutate and a PostToolUse compile keeps the manifest coherent and raises a drift alarm. Two verbs survive for the operations native tools can't do — write-mode (durable mode change) and unlock (temporary read-only exception) — plus check-write as hook plumbing. Mode is enforced by the prefix invariant evaluated in Go from the tool payload + disk. Decides the write-side-enforcement item deferred by ADR-0025 and retires its write skill; revisits ADR-0004. Enforcement is an explicit cooperative-agent guardrail, fail-closed on internal hook error but irreducibly fail-open on hook absence — drift detection, not the hook, is the integrity floor."
---

# ADR-0031 — Remove the `write` verb: native writes under hook-based mode enforcement

## Decision

The `memento write` verb is **removed entirely**. There is no tool-mediated write
path. Agents write note bodies with their **native tools** (Claude: Write / Edit /
MultiEdit / Bash redirects; codex: `apply_patch` / shell). Mode enforcement and
manifest coherence move to two harness hooks:

- **PreToolUse → `memento check-write`**: classifies the native operation, resolves
  the target, and returns an allow/deny verdict **before** the write touches disk.
- **PostToolUse → `memento compile`**: rebuilds the manifest/summary state and
  raises a drift alarm if the bytes that landed disagree with what was gated.

Three verbs make up the surviving surface — only the operations native tools have
no equivalent for keep a verb:

- **`write-mode <key> <append-only|living|read-only> --justification <reason>`** —
  durable mode change. Loosening requires `--justification`; tightening accepts it
  as optional self-documentation. Unknown mode values are **rejected**, not
  defaulted (closes ADR-0015's typo open-question).
- **`unlock <key> --justification <reason>`** — a temporary, single-key exception
  that re-opens the edit window on a read-only note until the next commit.
- **`check-write`** — hook-facing **plumbing** (not an agent verb); see contract
  below.

Driving principle (the conclusion of ADR-0015's friction-bypass argument): do not
fight the agent's native-write muscle memory unless a verb adds value it can't get
natively. For body prose it doesn't — and once writes bypass the verb, every other
guarantee bypasses with it. For *frontmatter mode operations* a verb does add
value (there is no native equivalent), so those keep verbs.

## What this supersedes / amends

- **ADR-0025 — write-side enforcement (its deferred item §43): decided.** Yes, add
  a `PreToolUse` gate — but it does **not** redirect native writes through
  `memento write` (now removed); it evaluates the mode lattice in `check-write` and
  vetoes in place. **Retires the write skill (ADR-0025 §27)**: its whole premise
  ("route writes through `memento write`") is deleted. The orient hook, lean-router
  orient, and pull-only `brief` from ADR-0025 are untouched.
- **ADR-0004 (v0 write scope): revisited.** The write verb it introduced is removed.
- **ADR-0022 (auto-compile after write): re-homed.** Recompile becomes the
  PostToolUse hook rather than a tail of the write verb. Its coherence intent
  survives; its enforcement intent does not (there is no write verb to tail).
- **Builds on** ADR-0015 (three-mode taxonomy — the only lattice), ADR-0017
  (ratification edit window — read-only bites only after first commit), ADR-0026
  (the A-UAT regime owns the validation gate below).

## Enforcement model

### The prefix invariant (the generative rule)

> **append-only ≡ new content has old content as a prefix.**
> **read-only ≡ new == old.** **living ≡ always allow.**

The shell operator matrix (`>>` append-safe; `>` truncate-unsafe) is a fast-path
special case of this invariant, not a separate rule.

### `check-write` is pre-mutate, and reads disk itself

`check-write` consumes the **raw PreToolUse JSON payload** on stdin, resolves the
absolute `file_path` to a vault-relative key, **reads old-bytes from disk itself**,
derives new-bytes, evaluates the invariant, and emits the harness verdict JSON
directly. The bash hook is a dumb pipe (`cat | memento check-write`); no lattice,
shell parsing, or message text is forked into bash or python — all of it is
unit-tested Go (extending ADR-0025's "verdict stays in Go" to its conclusion).

Mode and ratification are read from **disk and git**, never from the manifest. The
manifest is derived state we rebuild rather than guard (so guarding the enforcement
path on it would be a coherence inversion). Ratification is the existing predicate:
`git ls-files --error-unmatch <key>` (non-git tree ⇒ ratified). The mutability
predicate is `mutable = (unratified edit window) ∪ (active unlock grant)`.

`check-write` returns **`{decision, reason_code, message}`** — not a bare allow/deny.
The `reason_code` drives the denial-UX taxonomy below; the rendered `message`
becomes `permissionDecisionReason`.

### New-bytes derivation — and the replay caveat

- **Write:** new = `content` verbatim. **Correction to the working agreement:** the
  payload-alone derivation is exact *only* for Write.
- **Edit / MultiEdit:** new is a function of `(disk-old, payload)` — `check-write`
  must read disk and **replay the tool's mutation algorithm in Go** (sequential
  edits, `replace_all`, abort-on-ambiguous-match, create-only-via-Write). Claude's
  apply-semantics are an **unpublished, version-drifting contract**; if our replay
  diverges, the verdict is computed on bytes that differ from what lands.
- **codex `apply_patch`:** parse the patch envelope to derive new-bytes (same
  derivable class as Claude's Edit).

Because PostToolUse carries no *enforcement* role, a replay divergence would
otherwise be a silent, permanent hole. **Therefore PostToolUse `compile` is
"coherence **+ drift alarm**"** (not pure coherence): it compares what landed
against the gated expectation and surfaces a loud signal on mismatch. This is the
detective backstop under the predictive gate.

### Bash: deny unless provably append — and honestly fail-open beyond that

For Bash, new-bytes is not statically derivable. The allow rule is narrow: **a
single command segment whose *only* reference to the target is a literal `>>`
redirect to a resolved vault path, with no other redirection or known mutator
touching it** (allowed on append-only/living, denied on read-only). Everything else
that resolves to a vault path is denied.

Stated plainly because the working agreement overstated it: this is **broad-deny
only over *recognisable* references, and fail-open on the rest.** Variable /
command-substitution / `eval` / interpreter indirection (`$F`, `$(...)`, `bash -c`,
`python -c`) can write the vault without the parser ever seeing a path; and `>>`
alone does not prove append in a compound (`cmd >> f > f` net-truncates). These are
documented limits of a guardrail, not gaps to be closed by a bigger parser.

### Drive-by mode-change defense

A body-write must not smuggle a permanent `mode:` change under cover of a temporary
`unlock`. `check-write` compares the **effective parsed mode** (default applied) of
old vs new bytes; **when the note already exists and is ratified, any change to the
effective mode through a body-write path is denied** even under an active grant.
Mode changes route through `write-mode` exclusively.

Two carve-outs the working agreement missed (taken literally it blocked note
creation): a **new note** (Write, old absent) and an **unratified** note may set
`mode:` freely — that is legitimate birth/authoring, not a drive-by. New-bytes that
fail frontmatter parse ⇒ **deny** (can't verify mode safety). For Bash the defense
is moot by construction: the only allowed Bash write is `>>`, which appends after
the body and cannot rewrite the leading frontmatter block.

### The `unlock` grant and its audit trail

The grant is a gitignored `.memento/` sidecar (`key → {justification, granted_at}`),
file-scoped in `.gitignore` (the manifest/config under `.memento/` stay tracked).
Lifetime is **until the next commit — identical to ratification, no TTL**; the grant
covers all writes to that key until commit and is then deleted. There is no
flip-back: committing re-ratifies and the grant is cleared, which *is* the lock-back
(for an already-ratified read-only note it is the **grant deletion**, not
ratification, doing the re-lock).

**Correction to the working agreement:** the justification is *not* automatically in
the commit trail — the sidecar is gitignored and commit-cleared. The **pre-commit
hook lifts pending grant justifications into a `Memento-Unlock:` commit trailer
before clearing them**, so the *why* survives. Without this step the claim is false;
with it, it holds.

## Denial UX — the productive wall

When `check-write` denies, the message must move the agent to the right next action,
not a retry or a stall. Every message: names the reason in the agent's terms, names
**exactly one** recovery path with `<key>` pre-filled, states **"the identical write
will be denied again"**, and encodes self-serve vs. ask-user.

| reason_code | fires when | recovery the agent reaches for |
|---|---|---|
| `read_only` | ratified read-only, any edit | **ask the user**, then `unlock` (one-off) or `write-mode … living` (durable) |
| `append_only_interior` | ratified append-only, prefix broken | re-do as an append (no verb) **or** `write-mode … living` |
| `append_only_overwrite` | ratified append-only, truncate/Write | append (`>>`/tail edit) **or** `write-mode … living` |
| `drive_by_mode_change` | `mode:` changed via a body-write path | split: body edit (no `mode:` line) + `write-mode` |
| `bash_opaque_write` | un-derivable Bash vault write | use Write/Edit, or an explicit `>>` |
| `unwritable_path` | ignored / `_memento/` / non-`.md` | choose a valid `.md` key |
| `vault_discovery_ambiguous` | multiple `.memento` markers | **ask** (not deny): set `MEMENTO_VAULT_ROOT` |

**Read-only thaw is the one move gated by the user.** read-only is a deliberate
freeze (accepted ADRs, closed records); loosening it to `living` is mechanically a
normal `write-mode --justification`, but the denial message and the writing
convention **instruct the agent to confirm with the user first**. append-only→living
stays self-serve — append-only is just the default, not a commitment. (This is an
instruction-layer gate, unenforceable by construction since the agent runs the verb;
it is deliberate friction on the most consequential loosening, not a security
control.)

Two facts that must appear in the agent-facing surfaces (they don't today):
**modes bite only after a note's first commit** (so first-draft authoring never
walls), and **new notes are created by native Write** (the gate allows it — the wall
taxonomy is not "every vault write is gated").

## Trust model and failure posture

**Mode enforcement is a cooperative-agent guardrail, not a security control.** It
runs in user-editable client config (`.claude/settings.json`, codex `config.toml`)
invoking user-controlled scripts; anything that can write the vault can delete the
hook. It binds neither the human editor, nor `git` operations, nor claude.ai web.
The honest framing the ADR commits to: *the hook is best-effort prevention;
`compile`/`doctor` drift detection is the actual integrity floor.*

The harness blocks **only** on `exit 2` or an explicit `permissionDecision:"deny"`.
Everything else — exit 1, crash, timeout, missing `python3`, missing/stale
`check-write` binary, uninstalled or mis-matched hook — **proceeds**. Two postures
follow:

- **Fail-closed on internal hook error.** The wrapper denies (not falls through) on
  any internal failure for an in-vault target — missing interpreter, `check-write`
  absent/erroring, unparseable payload, ambiguous Bash. This buys back the old
  verb's loud-failure property where the harness allows it. (The current
  `set -euo pipefail` script fails *open* to exit 1 — that is a bug this ADR fixes.)
- **Fail-open on hook absence is irreducible.** No harness primitive says "block all
  writes unless an approving hook fired." If the hook never runs, nothing runs. The
  only user-facing signal that enforcement is live is `doctor` — which is why the
  doctor liveness checks below are a hard dependency of this change, not a nicety.

## Multi-agent

**Codex is in scope for enforcement, not just adherence.** This reverses ADR-0025's
"codex = skill-only, no hooks" premise, which is now stale: `codex-cli` (≥ 0.142)
ships a lifecycle hooks engine — `PreToolUse` / `PermissionRequest` / `PostToolUse`,
matcher groups, synchronous shell command, JSON-on-stdin → JSON-on-stdout — whose
deny contract is **byte-identical to Claude Code**
(`{"hookSpecificOutput":{"permissionDecision":"deny","permissionDecisionReason":…}}`).
Verified empirically: codex headless edits via `apply_patch` for structured writes
and raw `>> ` for appends.

**Correction (memento-ryr.39, codex-cli 0.142.2 — empirical, not the same two
buckets as Claude).** codex enforcement is **apply_patch-only**: the PreToolUse hook
**never fires for shell tools** in `codex exec`, so a shell write to a walled note
(`printf … > note.md`) runs completely **ungated**. codex matches EXACT tool names
(not regex — `.*`/`Shell` are dead), and its shell-approval surface
(`PermissionRequest`/exec-approval) is approval-facing and inert in exec mode
(*"exec command approval is not supported in exec mode"*), not a PreToolUse gate.
After the ryr.37 `reason_code` fix closes the `apply_patch` path (the bulk of codex
writes), codex write-enforcement remains **PARTIAL** on this version — a genuine
codex-cli limit, not a memento bug. Mitigation for codex shell writes is an external
read-only sandbox, not a hook. So codex's gate story is **apply_patch covered, shell
ungated**, which is weaker than Claude's two-bucket (Write/Edit + Bash) coverage.
This is the honest framing for this branch's codex-readiness call and feeds the parked
structural decision memento-ryr.35. (Matcher-string fix in `init` is
memento-ryr.40; this is the contract pin: see
[[codex-cli lifecycle hooks contract]].)

Two codex-specific install wrinkles `init` must own: codex's **hook-trust model**
(hooks trusted by hash, skipped until reviewed — so `init` cannot silently install a
*live* gate; the user must trust it) and **`apply_patch` envelope parsing** in
`check-write`. `init` gains per-agent-family branching (none exists today — it is
Claude-only) installing both hooks per family; the additive invariant of ADR-0025
holds — a family we can't detect or install degrades discoverability, never the CLI.

## Validation gate (owned by ADR-0026)

The entire trust model now rests on hooks firing, so this is empirical. Build a dev
branch and run the **write-verb build vs. the hooks-only build side by side** on the
A-UAT harness. Pre-registered claims, all 3/3 unless noted, gating the merge:

- read-only ratified note: native edit **denied** (Claude **and** codex).
- operator matrix: `>>` append allowed on append-only; `>`/Write denied.
- append-only: interior Edit denied, tail-append allowed.
- drive-by `mode:`→living under active `unlock`: **denied**.
- PostToolUse compile fires **only** on vault-internal writes.
- fail-closed self-test: remove `python3` / rename `check-write` ⇒ write **blocked**,
  not allowed.
- end-to-end: hooks-only build's read-only-leak rate ≤ the write-verb build's.

**Latency gates** (compile is now per-write, not a manual verb): tighten
`compile_bench_test.go` to a per-invocation budget, and add a `check-write` latency
gate — note `check-write` shells `git ls-files` per call for ratification on top of
process cold-start, the larger hot-path cost the working agreement undercounted.

## Consequences

- **CLI surface:** −1 verb (`write`), +2 porcelain (`write-mode`, `unlock`), +1
  plumbing (`check-write`, not listed in top-level help).
- **Go blast radius:** delete `runWrite` / the mutating IO in `internal/note/write.go`
  and the dead `OperationSectionReplace` / `OperationKeyedUpsert`; salvage
  `normalizeWritableKey` (path/key guarantees), `validateWriteMode` (the lattice),
  and the symlink-resolution of `writablePath` (**minus its `MkdirAll` side-effect**
  — a verdict must not mutate the filesystem) into `check-write`. Remove
  `ModeSectionReplace` / `ModeKeyedUpsert` and fix `validMode` to the three-node set
  — a **data-format tightening**, not just dead code: pre-existing notes carrying a
  retired mode flip to `frontmatter-invalid`, so grep the live vault first.
- **`init` / migration:** install the two hooks per family; rewrite the dormant
  broad-deny `pre-write-vault-guard.sh` into a `check-write` delegate with the new
  deny messages; remove the write skill. Old vaults carry orphaned artifacts
  (installed write skill instructing a dead verb; a tester who opted into the
  broad-deny guard is left with an **un-writable vault** once `write` is gone). Since
  `init` is additive-only by contract, **`doctor --fix` owns deletion** of orphaned
  skill + legacy hook entries.
- **`doctor` (hard dependency):** PreToolUse gate present, matcher correct, command
  path resolvable + executable; `check-write` on PATH and current; interpreter deps
  present; a **live-fire self-test** (synthesise a PreToolUse payload for a known
  read-only note, assert deny); a stale-grant check; and a blunt
  **"vault write enforcement: LIVE / OFF (reason)"** headline — because the failure
  is quiet, the status must be loud.
- **Observability:** a structured, gitignored `check-write` decision log (denials,
  grant consumptions, drive-by blocks) — the audit the old verb got for free via its
  commit trail.
- **Net trade-off (eyes open, corrected):** enforcement moves from a self-contained,
  synchronous, unit-tested verb to a distributed chain that is **fail-open by default
  on the whole surface** unless the fail-closed-on-error mitigation is built. Friction
  and adoption improve; debuggability and the failure mode get worse. Accepted, with
  the mitigations above, in exchange for not driving writes around the tool.

## Open questions / deferred

- **`create`/`new` verb — deferred, not rejected.** New notes are made by native
  Write today; template instantiation (`memento create <key> --template adr`) is a
  place a verb adds value native tools can't, consistent with the survival principle.
- **Incremental / coalesced compile.** PostToolUse recompile is O(whole-vault) per
  write with no incremental path. v0 ships full recompile with a tightened bench;
  debounce/incremental is the real fix if the hot path bites.
- **PostToolUse path-gating for Bash.** The post-hoc payload does not carry the
  written path, so "compile only on vault writes, same gate as PreToolUse" is not
  fully achievable for Bash — flagged as an open problem, not a confirmation.
- **Frontmatter reads (orthogonal).** An optional `read <key>#…` analog for
  frontmatter fields is not part of this decision.

## Addendum (2026-06-28) — ratification-boundary diff-audit backstop

The "fail-open by default unless the fail-closed-on-error mitigation is built"
trade-off above is leakier than the gate alone can fix: A-UAT proved three write
paths fail open **silently** (no deny, no drift alarm, no log) — codex `exec`
shell writes (codex fires PreToolUse only for `apply_patch`), untrusted codex
hooks (the whole gate no-ops until trusted), and Claude opaque shell
(`$(...)`/`eval`/`python -c`). All three are at the **tool boundary**, which is
inherently path-specific.

The catch-all is a **post-hoc audit of the on-disk diff against ratified (git
HEAD) state.** The diff is path-agnostic — it inspects end-state, so it does not
care how the bytes arrived. It reuses the existing pure verdict
`EvaluatePrefixInvariant`; the only new capability is sourcing `old` from **git
HEAD** rather than the pending-writes ledger. For each ratified note (present at
HEAD) changed on disk it recomputes the verdict with `old = HEAD`, `new = disk`,
honouring the same authorisation context check-write composes — an active unlock
grant waives it, and a pure `write-mode` change (only the mode: line differs,
isolated by re-normalising both sides to one mode) is exempt. Brand-new notes
(absent at HEAD) and compile's own manifest/brief rewrites are excluded by
auditing only ratified writable note keys. A violation with no covering grant
emits a NEW, distinct alarm class — `memento compile: MODE VIOLATION: <key> …` —
separate from the gated-handshake `DRIFT ALARM`.

It is hosted in `memento compile`, so it fires in **two tiers**: the PostToolUse
compile (early, where the hook fires) and — the true catch-all — the **git
pre-commit hook**, which already runs `memento compile`. The pre-commit anchor is
a git hook, independent of codex's lifecycle-hook trust model, sitting exactly at
**ratification**: it stops an unauthorised change from becoming permanent even if
it briefly lived on disk. Default is **DETECTION** (loud alarm; compile stays
exit-0 like the drift alarm so it never breaks an unrelated compile);
**MITIGATION** (exit non-zero → block the commit) is opt-in behind
`MEMENTO_STRICT_COMMIT`, default off because nothing is ratified pre-commit and a
surprise hard failure is worse than the alarm.

Posture shift: "codex enforcement is `apply_patch`-scoped" stops being a silent
gap and becomes **`apply_patch` gated in real-time, everything else caught at
commit.** Honest limits: commit-time not real-time; `git commit --no-verify`
skips it; uncommitted unauthorised edits linger until commit (the PostToolUse
tier catches earlier where it fires). See
[[enforcement backstop — ratification-boundary diff audit]].

## Related

- [[enforcement backstop — ratification-boundary diff audit]] — the learning note
  this addendum implements: the gap analysis and the why behind the pre-commit anchor.
- [[adr-0025-agent-integration-orient-hook-and-write-skill]] — decides its deferred
  write-side-enforcement item and retires its write skill; its orient hook stands.
- [[adr-0015-write-mode-taxonomy]] — the three-mode lattice this enforces; this ADR
  removes its retired-constant residue.
- [[adr-0017-pre-commit-edit-window]] — ratification: read-only bites only after
  first commit; the `unlock` grant reuses this edit-window concept.
- [[adr-0022-auto-compile-after-write]] — recompile, re-homed onto PostToolUse.
- [[adr-0004-v0-write-scope]] — the write verb this removes.
- [[adr-0026-agent-uat-validation-regime]] — owns the A/B validation gate.
- [[doctor-scoping]] — the enforcement-liveness checks this change makes mandatory.
