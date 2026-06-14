---
title: Write-mode taxonomy — three modes, default append-only, living unconstrained
status: accepted
mode: read-only
date: 2026-06-14
tags:
  - memento
  - write
  - mode
summary: "Write-mode collapses from four to three: `append-only`, `living`, `read-only`. `section-replace` and `keyed-upsert` retired before shipping. Docs without a declared `mode:` default to `append-only`. `living` mode currently imposes no API-level enforcement — full-file writes are allowed — pending real-world evidence that silent-drift is a problem worth solving with arcana."
---

# ADR-0015 — Write-mode taxonomy: three modes, default append-only, living unconstrained

## Decision

The write-mode taxonomy in spec §8 collapses from four modes to three.

| Mode | Semantics | Typical use |
|---|---|---|
| `append-only` | New content tacked on the end; existing content never rewritten | Logs, decision journals, ADR history. **Default when `mode:` is absent.** |
| `living` | Writes are allowed without API-level shape constraints | Discoveries, constraints, reference docs that evolve in place |
| `read-only` | Tool refuses to write | Accepted ADRs, frozen specs |

Three further commitments:

1. **`section-replace` and `keyed-upsert` are retired.** They appear in spec §8 as v2 modes but never shipped. The data-shape distinction they encoded (prose sections vs structured entries) is a content concern, not a mode dimension.
2. **Default mode is `append-only`.** When a doc has no `mode:` field in frontmatter, the write path treats it as `append-only`. Conservative default — agents may extend a doc but not rewrite it without an explicit opt-in from the human author. Per ADR-0017, the default mode binds after first commit; uncommitted notes remain in their edit window.
3. **`living` imposes no write-shape constraints.** Whole-file overwrites are permitted. The tool does not require the agent to name a section, batch edits, or otherwise structure its writes. This is a deliberate, time-bounded concession to friction reduction (see Context).

## Context

The previous taxonomy used `section-replace` (overwrite a named heading's section) and `keyed-upsert` ("add or update structured entries by key"). Two problems:

- **The names were database vocabulary in a markdown-prose surface.** `upsert` is SQL; `keyed` reads as record-oriented. A human curating their own knowledge vault sees four modes and has to puzzle out which one applies.
- **The two modes were probably one mode wearing different clothes.** Both target a named chunk and replace it. The distinction lay in what the chunk was — a prose section under an H2, or a list-entry under some key — and that is the writer's content choice, not a tool-level capability.

The collapsed three-mode set reads as patterns a human writer recognises: append, living, frozen.

The harder question was whether `living` should enforce a structured edit API (e.g., batched section-replace via a JSON op list) or allow free-form whole-file writes. The structured API has a real safety argument:

- Bounded blast radius — the tool only ever rewrites bytes inside the named section.
- *Mechanical* enforcement, not social — the gate makes silent loss impossible, rather than rare.
- Reuses the same value prop that justifies `read-only` (physical impossibility through the tool).

But there is a cost the safety argument did not weigh:

> An enforcement schema that is too constrictive and arcane will push the agent to bypass the tool entirely and edit files directly with user blessing. The tool that is technically safer but actually unused is strictly worse than the tool that is technically looser but actually drives writes through the gate.

The named-section + batched-edits API is a real instance of that risk. Composing a JSON edit list to update three sections plus add a fourth is enough friction that an agent (and a human reviewing one) will frequently elect to skip the tool and just edit the markdown. Once writes routinely bypass the tool, every other guarantee the write-path provides (mode enforcement on `read-only`, append discipline, future write triggers) is also bypassed. The mode system loses its load-bearing position.

Three alternatives were considered for `living` and rejected:

- **Whole-file write with a diff guard** (block if "too much" changed). Heuristic-driven; brittle; noisy. Rejected.
- **Batched named-section edits, required.** The safety-purist option. Rejected for the reason above: friction risk outweighs the loss-prevention benefit *until evidence of real loss appears*.
- **Allow whole-file but warn loudly on large diffs.** A soft middle. Rejected for now as solving a problem that has not been observed; can be added later if drift is a real failure mode.

`living` therefore ships as: writes allowed, tool does not constrain shape. Drift becomes visible through diffable-is-auditable (spec §14) and PR review — the same boundary the rest of the design relies on for catching silent rot.

## Consequences

- v0 ships **`append-only`** (default) **and `read-only`** as the only modes the write path enforces. This matches ADR-0004 (v0 write scope) — create plus append, mode-aware refusal for `read-only`. In v1, `read-only` refusal applies only after first commit; unratified read-only notes remain appendable until ratified.
- v2 adds **`living`** as the editable-doc mode. Its initial implementation is permissive: any write replaces the file body. No batched-edit API to design or implement.
- The mode system's load-bearing role narrows in v0 to `read-only` enforcement plus append discipline. That is enough to justify the schema; `living`'s contribution is opt-in *unlocking* of writes, not opt-in *structuring* of them.
- Reduced cardinality (3 vs 4) is easier to teach. The mode names are choosable from intuition: agents add to logs → `append-only`; humans evolve a reference doc → `living`; an accepted decision is frozen → `read-only`.
- Default = `append-only` means absent-frontmatter docs are conservatively writeable. New docs created without frontmatter inherit append discipline until the author opts in to `living`.
- The safety property previously promised by `section-replace` (bounded blast radius, mechanically enforced) is **deferred**. If real-world `living` use produces silent-drift incidents — agent fabricates a paragraph, agent quietly drops a constraint — revisit with batched-section-edits as a v3+ refinement. Until then, do not pre-build the enforcement.

## Open questions

- **When to revisit named-section enforcement for `living`.** A concrete trigger: more than a few observed incidents of agents losing or fabricating content in a `living` doc that diff review failed to catch. Until then, the cost of arcana outweighs the unrealised benefit.
- **`mode:` value validation.** A typo (`mode: appned-only`) currently falls through to the absent-`mode:` default, which would now mean `append-only`. The typo therefore lands somewhere reasonable, which masks the bug. A small parse-time warning ("unknown mode value: X") is a low-cost addition; out of v0 scope.
- **Append semantics for empty docs.** Creating a new doc and immediately appending to it is operationally indistinguishable from creating with content. The default-append-only path should not treat the first write specially. Worth a test, not worth an ADR.

## Supersedes (partial)

- Spec §8 mode table: the four-row table is replaced by the three-row table above. The retired modes (`section-replace`, `keyed-upsert`) appear in spec §8 as v2 features and are now retired before shipping. A future spec refresh removes them; this ADR pins the decision in the interim.
