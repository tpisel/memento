---
title: "Summary staleness lives in the ledger, not in frontmatter"
status: accepted
mode: read-only
date: 2026-06-17
tags:
  - memento
  - manifest
  - compile
  - staleness
  - summary
  - frontmatter
summary: "Per-doc summary-staleness state moves from frontmatter `summary_hash` into the canonical machine ledger (`.memento/manifest.json`). Each entry gains two carried hashes (`body_sha`, `summary_sha`); compile derives a three-state flag (`current`, `stale`, `missing`) deterministically from them. Brief renders staleness markers inline plus a footer count; `read` stderr emits the state alongside `binding:` and the link surface. No new verbs, flags, or explicit-ack actions are introduced — refresh falls out of natural edits via the two-hash rule."
---

# ADR-0023 — Summary staleness lives in the ledger

## Decision

Summary-staleness state is tool-owned and stored in the canonical machine ledger (`.memento/manifest.json`), not in per-doc frontmatter. The `schema_version` of `manifest.json` bumps to `2` for this change.

### Ledger entry shape

Per manifest entry, two hashes are carried alongside the existing derived fields:

- `body_sha` — sha256 of the post-frontmatter body, recomputed every compile.
- `summary_sha` — sha256 of the resolved summary text (frontmatter `summary:`, falling back to `description:`), recomputed every compile.

These sit beside the existing per-entry fields (`title`, `tags`, `headings`, `mode`, `links`, …) under each entry object. The boolean `summary_stale` field is removed; the derived `summary_state` enum below replaces it.

### Three-state model

Compile derives a per-entry `summary_state` from the just-walked vault and the prior ledger:

| State | Condition | Brief surface |
|---|---|---|
| `current` | summary text present and ledger's recorded `summary_sha` matches the just-computed `summary_sha`; `body_sha` matches the ledger's recorded `body_sha` | render summary, no marker |
| `stale` | summary text present and `summary_sha` matches the ledger, but `body_sha` differs | render summary with inline staleness marker |
| `missing` | no `summary:` or `description:` in frontmatter (regardless of first-paragraph fallback availability) | render first-paragraph fallback with `unsummarised` marker, or omit the line entirely if first paragraph is also empty |

### Refresh rule

A single rule covers every refresh path:

- **`summary_sha` changed since last compile** → write back both current hashes → `current`.  
  Covers: first-time summary write, summary typo fix, full living-doc rewrite, summary edit alongside body edit.
- **`body_sha` changed but `summary_sha` did not** → leave the recorded `summary_sha` in place, refresh `body_sha`? No — leave both as they were so a future "did the summary text change" check works. → `stale`.
- **Neither hash changed** → `current`.
- **No summary text at all** → `missing`. No ledger hashes recorded until a summary is authored.

A first-time compile after this ADR ships seeds the ledger from the current file state: any entry with a `summary:` (or `description:`) becomes `current`; any entry without becomes `missing`. No prior frontmatter `summary_hash` is read during seeding (see Migration).

No explicit-ack verb or flag is introduced. The "stale but summary still accurate" case is rare; when it arises, a trivial summary refresh-edit is the right action.

### Compile-time derivation and cost

Staleness derivation lives entirely in compile. The added cost is two sha256s and two map lookups per file. The body hash was already paid for by the existing walk; the summary hash is a single small string. Compile remains deterministic, pure, fast, and hook-safe (ADR-0006). Auto-compile-after-write (ADR-0022) and the pre-commit hook (ADR-0005) keep the ledger fresh at every meaningful session and commit boundary.

### Surfacing

- **Brief**: per-entry inline marker on the metadata line for `stale` and `missing` entries. Footer adds a counter line (`Staleness: N stale, M unsummarised`) so a scanner reads the aggregate without inspecting every entry. The exact glyph / token (`⚠ stale-summary`, `[stale]`, etc.) is a render-contract detail deferred to the brief work.
- **Read stderr**: a `summary: current|stale|missing` line emits alongside `binding:` and the link surface (ADR-0021), so an agent reading a single note gets the flag in context without round-tripping through brief.

### Migration

Existing frontmatter `summary_hash` becomes legacy-ignored. No formal migration verb is shipped — the only current consumer is this project's own dogfooding vault, and the file's body hash will reseed cleanly on first post-upgrade compile. External adopters are not yet in scope; if a future ADR introduces them, it may also introduce a formal migration path.

The YAML parser continues to tolerate the `summary_hash` field per ADR-0014's unknown-field policy. Authors may strip it at will. A future ADR may remove the legacy field name from the parser's known set entirely.

### Unratified files

Staleness applies normally. Edit-window status (ADR-0017) is orthogonal — a brand-new note legitimately surfaces as `missing` until a `summary:` is authored, which is a useful nudge rather than noise.

### Renames and deletion

Ledger entries follow the manifest entry's path key. On rename, the old entry is GC'd on the next compile and a new entry is recorded; if the body did not change, the new entry's first compile records the current state as fresh. On deletion, the ledger entry is dropped. No retention policy.

## Context

Spec §9 introduced `summary_hash` in per-doc frontmatter as the staleness commitment: "this summary describes a body with this hash." The location was inherited as a default rather than chosen by argument; the load-bearing design call — *trigger is body-content hash, not mtime* — is preserved here.

In-situ usage surfaced concrete costs of the frontmatter location:

- **Agent confusion.** Existing notes carry `summary_hash` in frontmatter, so agents authoring new notes had to guess whether to populate it themselves. The honest answer is "no, the tool owns it" — but if the field is human-visible and human-editable, every new author has to learn that out of band.
- **Schema-tier mismatch.** ADR-0014 splits frontmatter into tool-consumed (human-declared, tool-acted-on) and convention (human-declared, tool-ignored). `summary_hash` was neither: it is tool-*owned*, written and read by the tool with no meaningful human interaction. Keeping it in frontmatter blurs the schema's invariant.
- **Two staleness axes collapsed.** The existing `summary_stale` boolean folded "no summary committed" and "stale summary" into one bit, losing a distinction that drives different remedies (write a summary vs review an existing one). The three-state model separates them.
- **OKF interop noise.** Less memento-specific tool field in frontmatter is incidentally better for ADR-0018's downstream export story.

The architectural move is small: tool-owned derived state belongs in the canonical machine ledger, not the human-curated surface. Frontmatter stays cleanly human-declarable; `manifest.json` carries the carried state alongside the derived state it already carries.

Two alternatives were weighed and rejected:

- **Keep `summary_hash` in frontmatter; fix the agent confusion via documentation.** Rejected. The field would remain on the human surface, requiring perpetual "do not author this" reinforcement in orient and writing.md. The cleaner fix is to remove the field from the surface entirely.
- **Introduce a `--ack` review action to refresh stale state without editing the summary.** Rejected as speculative interface surface with no usage evidence. The two-hash refresh rule already handles every real path via natural edits. The "stale but summary still accurate" case is rare; when it does occur, a trivial summary refresh-edit is the right action, and is itself a hint that the prior summary may have been too generic. If practice later shows bulk acks-without-edit are a real need, the action can be added with evidence in a follow-up ADR.

## Consequences

- The frontmatter schema invariant tightens: every Tier 1 field is human-declarable.
- Orient and writing.md lose a "do not author `summary_hash`" footnote they would otherwise have needed forever.
- Brief surfaces a signal it could not surface before (`missing` ≠ `stale`).
- `read`'s stderr metadata channel grows by one line; consistent with the channel's role and ADR-0021's framing.
- `manifest.json` grows by ~150 bytes per entry for the two hashes. Negligible for any realistic vault.
- Compile remains deterministic, pure, fast, and hook-safe (ADR-0006). The ledger update is a pure function of vault content + prior ledger state.
- A property already true of `manifest.json` becomes explicit: it carries carried-state, not only derived-state. The single-file machine ledger is the right home for both.
- `schema_version` bumps to `2`; the `summary_stale` boolean per entry is replaced by `summary_state` and the two new hash fields. Consumers reading the old `summary_stale` are not preserved — there are none outside memento itself.
- Migration for the dogfooding vault is informal: the first post-upgrade compile reseeds. External adopters are not yet in scope.

## Open questions

- **Explicit ack action.** Deferred. If real workflow evidence shows bulk acks-without-edit are common, a review-verb action can be introduced. Until then, refresh is via summary edit.
- **Brief marker glyph and wording.** A render-contract detail; the brief-polish bead (`memento-ap3`) settles it when the rendering changes land.
- **Eventual removal of `summary_hash` from the frontmatter parser's known fields.** Currently tolerated under the unknown-field policy. A future ADR may remove the explicit `summaryHash` slot from the parser struct once adoption beyond the dogfooding vault is in view.

## Amends (partial)

- **ADR-0014 — Canonical frontmatter vocabulary.** `summary_hash` is removed from the Tier 1 table. The field is no longer tool-consumed; it is tool-owned and lives outside frontmatter. Legacy values in existing files are tolerated under the unknown-field policy.
- **ADR-0006 — Review verb and agent-assisted maintenance.** The review verb's worklist reads `summary_state` (and the underlying ledger entries) directly. No new action is introduced by this ADR; "flag missing or stale summaries" becomes the concrete "render the ledger's `stale` and `missing` counts and offer the matching keys."

## Related

- [[adr-0021-read-time-link-surface]] — staleness emits on the same stderr metadata channel as `binding:` and the link surface.
- [[adr-0022-auto-compile-after-write]] — the in-session correctness guarantee that keeps the ledger fresh between commits.
- [[adr-0017-pre-commit-edit-window]] — orthogonal: unratified files surface staleness normally.
- [[adr-0018-okf-compatible-frontmatter]] — moving `summary_hash` out of frontmatter incidentally reduces OKF-noise.
- Spec §9 — the "body-content hash, not mtime" trigger is preserved; this ADR moves *where* the hash lives, not how it's computed.
