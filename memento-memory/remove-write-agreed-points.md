---
title: Remove `write` — agreed points
summary: "Working agreement, pre-ADR: the `write` verb is removed entirely. Native agent tools own body prose; hooks become the enforcement layer (`check-write` pre-verdict + recompile post); typed verbs survive only for frontmatter mode ops (`write-mode --justification`) and self-expiring exceptions (`unlock --justification`). Captures the settled core so the eventual ADR doesn't re-litigate it; ramifications still to explore."
tags:
  - memento
  - write
  - mode
  - hooks
  - roadmap
  - open-question
mode: living
status: proposal
date: 2026-06-26
---

# Remove `write` — agreed points

> **Promoted to [[adr-0031-remove-write-verb-hook-enforced-native-writes]] (2026-06-26).**
> This is the historical pre-ADR working agreement. The ADR is authoritative and
> carries the corrections that six domains of ramification-exploration surfaced —
> notably: the `check-write <key> --op` signature was wrong (it consumes the tool
> payload), new-bytes is only payload-derivable for Write, the Bash rule is honestly
> fail-open beyond explicit `>>`, the drive-by defense needs a create/unratified
> carve-out, the unlock justification needs a pre-commit trailer to reach git, and
> **codex has hooks now** (ADR-0025's "codex = skill-only" premise is stale), so
> enforcement is multi-agent. Read the ADR, not this, for the settled design.

Working agreement reached in design discussion. Purpose: pin the core decisions so we
could explore ramifications without re-arguing the foundation. Supersedes the
write-side-enforcement arm of [[adr-0025-agent-integration-orient-hook-and-write-skill]];
revisits [[adr-0004-v0-write-scope]] and builds on [[adr-0015-write-mode-taxonomy]].

## Core decision

The `memento write` verb is **removed entirely**. There is no tool-mediated
write path. Agents write body content with their **native tools** (Write / Edit /
MultiEdit / Bash redirects); enforcement and coherence move to **hooks**.

Driving principle: don't fight the agent's native-write muscle memory unless the
verb adds value it can't get natively. For body prose it doesn't (ADR-0015's
friction-bypass argument, taken to its conclusion). It still does for *frontmatter
mode operations*, which have no native muscle-memory equivalent — so those keep
verbs.

## Agreed points

1. **Remove `write` fully** — not demote to an optional append path. No
   tool-mediated append survives; append-only rests entirely on the hook
   (point 4). Chosen deliberately over keeping a safe-append power-path.

2. **Derived state is rebuilt, not guarded.** The manifest is reconstructable by
   `compile`. Coherence is maintained by a **PostToolUse recompile**, not by
   intercepting writes. `doctor`/`compile` drift-detection is the catch-up path
   when a write slips the hooks. See [[doctor-scoping]] ("manifest freshness").

3. **Enforcement = two hooks, verdict stays in Go.**
   - **PreToolUse:** classify the native operation, resolve the target path, then
     ask `memento check-write` for an allow/deny verdict. The mode lattice +
     ratification logic is **not** forked into bash — `check-write` is the single
     source of truth.
   - **PostToolUse:** run `compile` to keep the manifest/summary state coherent.

4. **append-only is mechanically enforced (option a), via the prefix invariant.**
   Generative rule:
   > **append-only ≡ new content has old content as a prefix.**
   > **read-only ≡ new == old.** **living ≡ always allow.**
   Hooks **differentiate operators** rather than blanket-deny:
   - `>>` (append) → allowed on append-only; `>` (truncate) and `Write` → denied.
   - `Edit`/`MultiEdit` → allowed on append-only **iff** the edit is a pure
     tail-append (prefix preserved); interior rewrites denied.
   - read-only → both denied. living → both allowed.
   The operator matrix is a fast-path special case of the prefix invariant.
   **The gate is fully pre-mutate** — the verdict is derivable *before* the write,
   not by mutate→detect→rollback:
   - **Structured tools (Write/Edit/MultiEdit):** new-bytes is derivable in memory
     from the tool payload (`Write.content`; or current file + `Edit` old/new
     string applied), so `check-write` evaluates the invariant as a pure function
     of (old-bytes, new-bytes) and denies before disk is touched.
   - **Bash:** new-bytes is *not* statically derivable (opaque command), so Bash is
     broad-deny on any vault write **except a syntactically explicit `>>`** to the
     target, which is provably append regardless of content (allowed on
     append-only/living, denied on read-only).
   This makes **PostToolUse purely a coherence step** (recompile); it carries no
   enforcement role.

5. **Surviving verbs (the only mutation porcelain):**
   - **`write-mode <key> <append-only|living|read-only> --justification <reason>`**
     — durable mode change. Loosening (read-only → append-only/living,
     append-only → living) **requires `--justification`**. Tightening does **not**
     require it, but `--justification` is **accepted** as an optional
     self-documentation affordance. Renamed from the old `--force-with-reason`.
     Name `write-mode` chosen for descriptiveness.
   - **`unlock <key> --justification <reason>`** — temporary, self-expiring,
     single-key exception (point 6). A **separate verb** from `write-mode`:
     granting an exception ≠ setting a durable mode, and we don't want the agent
     conflating them.

6. **Temporary relax re-opens the edit window — it does not toggle `mode:`.**
   - Reuses the existing unratified concept. `check-write`'s mutability predicate
     becomes `mutable now = (unratified edit window) ∪ (active grant)`.
   - **The lease lasts until the next commit — full stop, identical to
     ratification's lifetime.** No TTL. The "never commits" risk is exactly the
     existing tolerated state of an unratified-but-uncommitted note, so we inherit
     it rather than invent a parallel expiry. This also settles consumption
     granularity: the grant covers **all writes to that key until commit**, not a
     single write.
   - Fail-safe by construction: default is locked, the grant dies at commit, and
     there is **no flip-back** — committing re-ratifies, which *is* the lock-back.
     This is why we reject the naive `mode:` off→on toggle (fail-unsafe,
     over-broad, diff churn).
   - A grant unlocks **body mutation only — never the `mode:` field itself**; see
     the drive-by defense in point 8.
   - Justification captured at grant time and surfaced into the commit trail
     (the *why*, not just "approved").
   - State lives in a **gitignored `.memento/` sidecar** (key → {justification,
     granted_at}); cleared on commit.

7. **Mode lattice is the three nodes from ADR-0015** — read-only / append-only /
   living. The retired `section-replace` / `keyed-upsert` constants are **dead
   code to remove** (`internal/markdown/metadata.go`, `internal/note/write.go`);
   `validMode()` currently still accepts them, contradicting ADR-0015.

8. **`check-write` — surface is the part most needing exploration.** It absorbs
   everything `write` guaranteed by being the only door. Candidate input set (to
   be pinned): `path`, `old-bytes`, `new-bytes`, `tool`/`actor`, `op`, and
   **`frontmatter-delta`**. Responsibilities:
   - op-gate (mode × operation, via the prefix invariant on old/new bytes);
   - key/path validation (ignored paths, `_memento/`, vault-prefix, non-`.md`);
   - ratification check (read-only only bites after first commit — ADR-0017);
   - grant predicate (active `unlock`);
   - **drive-by mode-change defense** (below).
   - **Conditional by tool:** Write/Edit/MultiEdit supply old/new bytes for an
     in-memory invariant check; Bash carries no derivable new-bytes and is
     **broad-deny unless a syntactically explicit `>>`** to the target (heredocs,
     `>|`, fd-dup, `eval`/`$VAR` indirection, `truncate`/`dd` all fall to deny).

   **Drive-by mode change — decided: we care, and we prevent it.** Scenario:
   read-only file → `unlock` → agent edits the body **and** flips the `mode:` line
   to `living` in the same native write, smuggling a *permanent* loosening under
   cover of a *temporary* grant, with no `write-mode --justification`. Defense:
   `check-write` inspects `frontmatter-delta`, and **any change to `mode:` through
   a body-write path is denied even under an active grant** — mode changes route
   through `write-mode` exclusively. This is why `frontmatter-delta` is a
   `check-write` input. (Fallback if we ever drop this: the change still surfaces
   in commit-diff review, ADR-0015's backstop — but mode is the *control* field,
   so mechanical prevention is cheap and proportionate.)

## Revised CLI surface

```
memento help
memento version
memento init [--dir <vault>]
memento compile [--quiet]                 # also the PostToolUse target
memento brief
memento orient
memento read <key|@N>[#<heading>]
memento convention <name>
memento write-mode <key> <append-only|living|read-only> --justification <reason>
memento unlock <key> --justification <reason>
memento check-write <key> --op <append|overwrite>     # plumbing, hook-facing
```

Net vs today: **−1** (`write`), **+2 porcelain** (`write-mode`, `unlock`),
**+1 plumbing** (`check-write`). Porcelain/plumbing split is explicit:
`check-write` is hook infrastructure, not an agent-invoked verb.

## Validation plan

- Build on a **dev branch** and run the **write-verb build vs the hooks-only
  build side by side** to compare. The entire enforcement trust model now rests
  on hooks firing reliably, so this is empirical, not assumed.
- Concretely confirm: a native `Edit` to a ratified read-only file is **blocked
  in both Claude and codex** before the architecture is committed to an ADR.
- **PostToolUse `compile` must fire only when the tool call wrote inside the
  memory vault** — same path gate as the PreToolUse hook — not on every repo edit.
  Confirm this.
- **Tighten the `compile` performance benchmark.** `compile` now runs in the
  write hot path (potentially many times per task), so its latency budget matters
  more than when it was a manual verb. Revisit `internal/cli/compile_bench_test.go`
  thresholds.

## Accepted trade-off (eyes open)

Enforcement moves from a self-contained, synchronous, unit-testable verb to a
**distributed, stateful chain** (bash → python → `check-write` Go → git
ratification state → `.memento/` grant sidecar → post-commit cleanup). Friction
and adoption improve; total moving parts and debuggability get worse, and the
failure mode gets quieter (a misconfigured verb errors loudly; a misconfigured
hook silently enforces nothing). Accepted in exchange for not driving writes
around the tool.

## Deferred — ramifications still to explore

- **A `create`/`new` verb, defer-but-document.** With no `write`, new notes are
  created by native Write using the canonical frontmatter template (commit
  `23159ec`) + the writing convention. But *template instantiation* is a place a
  verb adds value native tools can't — so we likely still want
  `memento create <key> --template adr` later (scaffold frontmatter, validate key
  at birth). This is **consistent with removing `write`**: the principle is
  "verbs survive where they add value," and scaffolding is value-add, not a
  body-content surface competing with Edit. Deferred, not rejected.
- **`check-write` input shape + hot-path cost** — pin the exact payload (point 8)
  and confirm latency/reliability when invoked per structured write tool call via
  shell → Go. This is the surface flagged as most needing exploration.
- **Summary:** no verb. `compile` already recomputes `SummaryTextHash` /
  `SummaryState`, so hand-editing `summary:` + recompile is correct. Confirm the
  staleness signal is enough of a prompt after a native body edit.
- **Frontmatter reads (orthogonal):** optional `read <key>#... ` analog for
  frontmatter fields (`key>field`); not core to this decision.
- **`--justification` typo/validation** — mode-value typos currently fall through
  to the append-only default (ADR-0015 open question); does `write-mode` reject
  unknown enum values?

## Rejected alternatives

- **Keep a mandatory `write` verb.** Loses to native-tool muscle memory; agents
  bypass it, and once writes bypass the tool every other guarantee bypasses with
  it (ADR-0015 §46–50).
- **Demote `write` to an optional safe-append path.** Rejected in favour of full
  removal (point 1).
- **Bake the mode×operation matrix into the bash hook.** Forks the lattice +
  ratification logic out of Go; they drift. Delegate to `check-write` instead.
- **Naive `mode:` off→on toggle for one-time edits.** Fail-unsafe, over-broad,
  diff churn. Replaced by `unlock` (point 6).
- **Snapshot → mutate → detect → rollback** for append-only. Rejected for a
  **pre-mutate** derivation: new-bytes is knowable before the write for structured
  tools, so we never need to write-then-undo (point 4).
- **Soft read-only via permission "ask".** Dilutes read-only from "refuse" to
  "confirm" and records no *why*. Kept the deliberate, logged, self-expiring
  `unlock` for read-only; "ask" may still be proportionate for append-only.
