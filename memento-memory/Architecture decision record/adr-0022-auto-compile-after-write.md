---
title: "Auto-compile after successful write"
status: accepted
mode: read-only
date: 2026-06-15
tags:
  - memento
  - write
  - compile
  - manifest
summary: "`memento write` runs `compile` after a successful body write. The recompile keeps the manifest and brief consistent with the in-session view of the vault, so a subsequent `memento read @N` or `memento brief` reflects the write without an explicit recompile step. The cost is bounded — `BenchmarkCompile500Docs` measures ~18ms on Apple M2 Max — and idempotent with the pre-commit hook. No `--no-compile` opt-out shipped; revisit if real batch-write friction appears."
---

# ADR-0022 — Auto-compile after successful write

## Decision

`memento write` runs a full vault recompile after every successful write, before returning to the caller. The recompile is the same `Compile()` call the CLI verb invokes — no special partial-update path.

Concretely:

- **Trigger:** a successful `write` body operation (append or overwrite, ratified or unratified). A `write` that fails its mode/ratification check does not trigger compile — there is nothing new to index.
- **Scope:** identical to the standalone `memento compile` verb. Both `.memento/manifest.json` and `_memento/brief.md` are regenerated.
- **Output:** the write's success line remains the verb's stdout contract. The recompile's outcome lands on stderr (e.g., `compiled: <N> entries` or whatever line `compile` already emits), suffixed after `binding:` and any per-ADR-0021 link surface. Compile errors surface as a separate, non-fatal warning — the write itself has already succeeded and the body is on disk.
- **No opt-out flag.** `memento write --no-compile` is **not** introduced. A batch-write workflow that wants to defer recompile can either invoke `memento compile` once at the end (the auto-compile is overhead but not harmful) or, if real friction emerges, motivate an explicit flag in a follow-up ADR.

## Context

Before this ADR, `memento write` mutated the body on disk but left `.memento/manifest.json` and `_memento/brief.md` stale until the next compile — either an explicit `memento compile` call or the pre-commit hook. The agent's in-session view of the vault therefore diverged from disk the moment it wrote anything: a `memento read @N` after a write could resolve to the wrong entry, and `memento brief` would not reflect the new note. The pre-commit hook closes the loop at commit boundary, but within a session the manifest is stale.

Three candidates were weighed:

- **Print a hint** — `memento write` succeeds and emits `hint: run 'memento compile' to refresh the manifest` on stderr. Cheap, transparent. Rejected because it requires the agent to remember an extra step *every write*; missing it produces silently-stale reads. The same kind of advisory-not-enforcement failure mode ADR-0017's edit-window predicate was designed to avoid.

- **Rely on the pre-commit hook only** — do nothing. Rejected because the pre-commit hook is a *boundary* refresh, not an *in-session* refresh. The hook makes commits diffable; it does not help an agent that writes, reads, writes, reads inside a single task loop.

- **Auto-compile only when the manifest is older than the written file's new mtime** — a guarded auto-compile. Rejected as premature optimisation. The full compile is fast enough that conditional logic adds complexity without measurable benefit, and the condition predicate itself can misfire on filesystem-mtime weirdness (the same class of problem spec §9 cites for summary-staleness's body-hash trigger).

The full-recompile-after-write position was chosen on the cost-vs-correctness ledger:

- **Cost is bounded.** `BenchmarkCompile500Docs` records ~18ms (22.2MB, 108k allocs) on Apple M2 Max for a 500-document synthetic vault. Real project vaults are typically tens of notes, not hundreds; their compile cost is sub-millisecond to single-digit milliseconds — well under the perceptual threshold for CLI latency, and trivially recovered by the next write being effectively-cached on the OS file cache. `TestCompileWithin1s` already gates the 500-doc fixture under 1 second.
- **Correctness is the whole point.** Stale manifest = wrong `@N` resolution, wrong brief snapshot, wrong post-write reasoning. The cost of being wrong vastly exceeds the cost of 18ms.
- **Idempotency is free.** The pre-commit hook still runs compile at commit time; an auto-compile on every write means the hook's compile is a no-op against an already-fresh manifest. No correctness conflict.

The reasoning generalises ADR-0017's "the hook is where review happens" framing: *the hook is the durability surface; in-session correctness needs its own mechanism, and auto-compile-after-write is it*.

## Consequences

- The agent's session model simplifies: after `memento write` succeeds, the manifest and brief reflect the write. No "remember to recompile" rule in orient or writing.md.
- Per-write latency includes a vault recompile. For project-sized vaults this is invisible; for synthetic large vaults it is sub-second. If a future vault exceeds the budget, the response is to optimise compile (the synthetic benchmark already exists as a gate) — not to disable auto-compile.
- The pre-commit hook's compile becomes redundant in the common case (the manifest is already fresh). It stays — it remains the diffability/auditability guarantee for unhooked or batched scenarios — and runs effectively free against a fresh manifest.
- `memento write`'s output contract gains a stderr line for the recompile result. Documented in spec §10's CLI surface and in orient's `write` description.
- Spec §15's "Post-write manifest/brief refresh guidance" deferred item is resolved.

## Open questions

- **Latency budget.** No hard cap is enforced. If a real-world vault makes `memento write` perceptibly slow, the response is to optimise compile (the synthetic-vault gate is the existing pressure point) before adding an opt-out flag.
- **Batch writes.** If a script or workflow does many `memento write` calls in sequence, the auto-compile runs each time. If this becomes a measured pain point, an explicit `--no-compile` flag (per-call) or a batch verb (e.g., `memento write --batch`) is the obvious response. Wait for evidence.
- **Compile-error handling.** A write that succeeds followed by a compile that fails is a partial-success state — the body is on disk, but the manifest is not refreshed. The chosen behaviour distinguishes this from a write failure: write failures return exit 1 and do not mutate the body; compile-after-write failures return exit 3, surface a stderr warning, and tell the caller to run `memento compile` to refresh the manifest. This keeps retrying append-mode writes from duplicating content when the only remaining work is a recoverable manifest refresh.
- **Auto-compile on `write` failure.** The ADR pins auto-compile to *successful* writes only. A failed write (mode rejected, ratification refused, I/O error) leaves the manifest as-is. This is the intended behaviour; flagged here to make the asymmetry explicit.

## Supersedes (partial)

- Spec §8 ("autonomous write with asynchronous review via git diff") — refined: in-session manifest consistency is now automatic, not advisory. The diff-review framing for *cross-session* review is unchanged.
- Spec §15 "Post-write manifest/brief refresh guidance" — resolved by this ADR (auto-compile, no flag, no hint).

## Related

- [[adr-0017-pre-commit-edit-window]] — the pre-commit hook is the cross-session durability surface; this ADR is the in-session one.
- [[adr-0008-memento-brief-projection]] — brief is part of compile's output; auto-compile refreshes it.
- Spec §4 / §9 — compile's purity and stateless-full-rebuild posture; this ADR does not change either.
