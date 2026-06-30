---
title: "mtime-gated lazy recompile on brief (and why read is not gated)"
status: accepted
mode: read-only
date: 2026-06-30
tags:
  - memento
  - brief
  - compile
  - manifest
  - freshness
  - adr
summary: "brief runs a cheap stat-only freshness gate and recompiles ONLY when a note's mtime is strictly newer than the manifest's; the already-fresh case is O(n) os.Stat calls with no body reads. This catches the drift the PostToolUse hook structurally misses — a human editing a note mid-session produces no agent tool write, so the manifest/brief would otherwise lag until the next explicit compile. The lazy path runs writeCompileArtifacts only, NOT the DRIFT ALARM / MODE VIOLATION audits, which stay on explicit compile + PostToolUse + pre-commit so integrity signals are never absorbed by a read-side verb. A missing manifest is out of scope (bootstrap, handled by readOrRenderBrief's compile hint), not a freshness case. read is deliberately NOT gated: it serves note bodies fresh from disk, so only its manifest-derived stderr metadata can lag — lower stakes — and gating the hottest verb is not worth the stat-walk while brief at task start already refreshes the shared manifest. Refines ADR-0022, which rejected guarded auto-compile in the write-trigger context that ADR-0031 has since removed."
---

# ADR-0033 — mtime-gated lazy recompile on `brief` (and why `read` is not gated)

## Decision

`memento brief` runs a freshness gate before serving its projection:

- `os.Stat` the manifest (`v.ManifestPath`). If it is **absent**, do nothing — that
  is a bootstrap condition handled downstream by `readOrRenderBrief` (render from an
  existing manifest, else the `manifest-not-found` compile hint), not a freshness
  case.
- Otherwise stat-walk the note corpus (`WalkMarkdown`, **stat only — no body
  reads**) and short-circuit at the first note whose mtime is **strictly newer**
  than the manifest's. Equal mtimes count as fresh; the manifest is written after
  the notes it indexes, so coarse-resolution ties must not force a recompile.
- If a newer note exists, run `writeCompileArtifacts` (manifest + brief) before
  rendering. Otherwise serve the existing artifacts unchanged.

The already-fresh case — the ~90% path — costs O(n) `os.Stat` calls and zero body
reads. `Marshal` is deterministic, so even a triggered recompile of an unchanged
vault yields byte-identical artifacts (no git churn).

**The lazy path runs coherence work only.** It calls `writeCompileArtifacts`, *not*
the full `compile` verb, so the **DRIFT ALARM** and **MODE VIOLATION** audits
(ADR-0031) do **not** fire on `brief`. Those integrity signals stay on explicit
`compile`, the PostToolUse hook, and pre-commit, so a read-side verb can never
silently absorb them.

**`read` is deliberately NOT gated.** See rationale below.

## Context

The removed `write` verb used to auto-compile after writing (ADR-0022). Under
ADR-0031 that responsibility moved to a PostToolUse `check-write`/`compile` hook,
which fires **only on the agent's own tool writes**. The most common real-world
drift — a human editing a note in Obsidian while the agent works — produces no
agent tool write, so the hook never fires and the manifest/brief go stale until the
next explicit `compile`, SessionStart, or pre-commit. `brief` is the verb an agent
hits at task start, so a stale brief there poisons the whole session's view.

A blanket "recompile on every `brief`" was rejected: `compile()` walks the entire
vault and re-reads every note, and `writeCompileArtifacts` unconditionally rewrites
both artifacts. `brief` is the hottest verb; paying a full vault re-walk and re-read
on every call — including the common already-fresh case — is the wrong default. The
mtime gate buys correctness for the cost of a stat-walk.

### Reconciling with ADR-0022

ADR-0022 explicitly **rejected** "auto-compile only when the manifest is older than
the written file's new mtime" as premature optimisation, citing filesystem-mtime
weirdness. That rejection was correct **in its context**: the trigger was a
*successful `write`*, which already knew it had mutated the vault, so the cheapest
correct action was an unconditional recompile and a guard only added misfire risk
for no saving.

That context no longer exists. ADR-0031 removed the `write` verb; there is no longer
a write event to hang an unconditional recompile on. The trigger is now a *read-side
verb with no knowledge of whether the vault changed*, where the only way to avoid a
full re-walk on every call **is** the mtime guard. The guard ADR-0022 rejected as
needless is, in the post-ADR-0031 model, the mechanism that makes catching
out-of-band edits affordable. The mtime caveats ADR-0022 raised remain real, which
is why this gate is **best-effort**: explicit `compile` and the hooks remain the
authoritative refresh, and ADR-0032's `doctor` manifest-freshness check is the
higher-fidelity (in-buffer recompile + diff) authority that shares this predicate.

### Why `read` is not gated

`read` was considered for the same gate and deliberately excluded:

- **`read` serves note bodies fresh from disk.** `note.Read` opens the actual file
  every call, so the primary content a `read` returns is never stale. Only the
  manifest-*derived* stderr metadata (binding, summary-state, link surface) can lag
  — strictly lower-stakes staleness than a stale brief projection.
- **`read` is even hotter than `brief`.** An agent issues many `read`s per task;
  adding a vault-wide stat-walk to each one taxes the highest-frequency verb to
  refresh secondary metadata.
- **`brief` already refreshes the shared manifest.** The orientation flow runs
  `brief` at task start; its gate recompiles the manifest that subsequent `read`s
  then draw their metadata from. A human edit *after* `brief` can still lag `read`'s
  metadata within a session, but that is the low-stakes case above, and the agent
  can re-run `brief` (or rely on `doctor`) to refresh.

If read-side metadata staleness proves to matter in practice, the gate helper is
already factored to be reusable; revisit then with evidence.

## Consequences

- An out-of-band note edit is reflected the next time the agent runs `brief`,
  without any explicit `compile`.
- The already-fresh `brief` stays a stat-only no-op: no manifest/brief rewrite, no
  git churn.
- `brief`'s output contract is unchanged in the fresh case; in the stale case it
  silently recompiles (warnings discarded — they still surface on explicit
  `compile` and the hooks) and keeps its clean stdout-only contract.
- A wholly-missing manifest keeps its existing behaviour (render-from-manifest or
  the `manifest-not-found` compile hint); this gate does not bootstrap.
- `read`'s manifest-derived metadata can lag a same-session out-of-band edit made
  after the last `brief`. Accepted as low-stakes (bodies are always fresh).

## Related

- [[adr-0022-auto-compile-after-write]] — rejected the mtime guard in the
  write-trigger context this ADR's read-trigger context inverts.
- [[adr-0031-remove-write-verb-hook-enforced-native-writes]] — moved auto-compile to
  the PostToolUse hook, creating the out-of-band-edit gap this gate closes; owns the
  DRIFT ALARM / MODE VIOLATION audits this lazy path stays clear of.
- [[adr-0032-doctor-scope-and-check-model]] — `doctor`'s manifest-freshness check is
  the authoritative (in-buffer recompile + diff) sibling of this best-effort gate.
- [[adr-0008-memento-brief-projection]] — brief is part of compile's output; this
  gate keeps it current.
