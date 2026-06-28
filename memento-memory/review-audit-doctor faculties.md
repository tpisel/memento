---
title: Review / audit / doctor — faculty boundaries
status: proposal
mode: append-only
date: 2026-06-25
tags: [memento, review, audit, doctor, roadmap, proposal, open-question]
summary: "A carve for three maintenance-shaped verbs that have been blurring together. `doctor` is already assigned the machine/config-health role (ADR-0025), which frees `audit` from the machinery it was straddling. The remaining cut is between `review` (closed-world: judge the vault against itself + world-facts — form, structure, internal consistency) and `audit` (open-world: judge the vault against an external referent — the human's intent and an untampered ground truth). \"Is each note well-formed / is the corpus tidy / is it self-consistent\" is review; \"is this the memory you actually want, and has it been silently rewritten\" is audit. Not decided — records the dividing principle so the review/audit/doctor verb ADRs don't each re-derive it."
---

# Review / audit / doctor — faculty boundaries

The load-bearing claim: **`review` is overbroad and `audit` was straddling two unlike things.** Once `doctor` takes the machinery, the clean split between what's left is *closed-world vs open-world* — whether the judgement can be made from the vault alone, or needs a referent outside it.

This note carves the boundary. It does not pin the verbs; it gives the three eventual ADRs (review-file reservation, audit verb, doctor verb) a shared dividing principle so each does not redesign the cut from scratch. Companion to [[agent-human review boundaries]] (which carves *who reviews at what cadence*, an orthogonal axis) and the tool-read-file pattern in ADR-0010.

## What is already decided

- **`doctor` = configuration/machine-state check.** ADR-0025 already distinguishes the `doctor` verb's "configuration-state check" from "`review`'s content check", and ADR-0010 routes presence/absence nudges (e.g. "this vault has no writing.md") to doctor. So doctor owns the *plumbing*: install/hook/skill state, manifest freshness, config validity, vault discoverability, ignore correctness, presence of expected tool-read files. Mechanical, no content judgement, no agent.
- This resolves half of the "does audit overlap doctor?" question: the machinery half audit seemed to want is already doctor's. Audit is freed to be purely epistemic.

## Why `review` is overbroad

Four distinct things have been collapsing into one verb:

1. **Form** — each note against style and frontmatter. (Mechanical; ADR-0006's mechanical review; arguably doctor-adjacent.)
2. **Structure** — the corpus as a whole: un-organised content, notes not pulling their weight, redundancy, missing decomposition. (Agent judgement.)
3. **Consistency / staleness** — factual contradictions, statements made earlier that are no longer true and should be pruned or de-weighted. (Agent judgement; ADR-0006 files "obsolete notes" here.)
4. **Strategic fit** — is the memory doing what you *want* it to do. (Agent + human judgement.)

These differ along one principled axis:

- **1–3 are closed-world.** You can decide them from the vault alone, plus general world-facts. Malformed (1), redundant (2), self-contradictory or superseded (3) are all detectable without knowing your goals.
- **4 is open-world.** "Doing what you want" requires a referent that is *not in the vault*: your intent, the project's actual needs. You cannot compute it from the notes — the giveaway that it is a different faculty.

## The carve

| Faculty | Question | Referent | Mode | Levels |
|---|---|---|---|---|
| **doctor** | Is the machinery healthy? | the tool's own expected state | mechanical | — (pre-assigned) |
| **review** | Is the memory well-formed, tidy, and self-consistent? | the vault + world-facts (closed-world) | mechanical + agent | 1, 2, 3 |
| **audit** | Is this the memory you want, and can you trust it? | the human's intent + untampered ground truth (open-world) | agent + human | 4 + integrity |

**Audit absorbs two things that belong together because both are open-world and both demand the human's god's-eye view:**

- **Strategic alignment** (level 4) — does the corpus still serve its purpose; what is missing; what is over-invested.
- **Integrity / inspectability** — the namesake risk from [[conditional-information-access]]: an externalised memory is a place where someone can rewrite your past without your knowing. Auditing that the projection is dumpable, reviewable, and untampered, and that invariants (the [[agent-human review boundaries]] "Invariants" rung — ADRs, spec, constraints) still hold. The agent structurally cannot self-audit this; it is the one faculty that is human-on-the-hook by construction.

## The seam to watch (level 3)

Staleness has two faces and the boundary runs through it:

- *Internal* staleness — note A contradicts newer note B, or an ADR superseded it. Closed-world → **review**.
- *External* staleness — the project moved on and the note no longer matches reality-out-there. Needs the external referent → leans **audit**.

Most "no longer relevant" cases present as the first and resolve in review; the hard residue (still internally consistent, but no longer what the project is about) is audit's. Don't force a hard line here — flag the case to the more powerful faculty.

## Tool-read-file implication (ADR-0010)

The pattern gives at most one `_memento/<verb>.md` per verb. Under this carve:

- `review.md` = local conventions spanning levels 1–3 (house style, frontmatter expectations, what "pulling its weight" means here, when to prune vs de-weight). One file; the verb may expose `--mechanical` / `--deep` modes rather than fracturing into four files.
- `audit.md` = the **rubric of intent**: what this vault is *for*, what good coverage looks like, which invariants are load-bearing — the external referent audit judges against. This is why audit.md cannot be auto-derived: it encodes goals the vault does not otherwise contain.
- `doctor` likely needs no tool-read file (nothing local to opine; it checks tool-defined state).

## Explicitly not decided

- Whether these are three verbs, or fewer verbs with modes. The faculties are real; the verb count is a packaging choice.
- Whether level 1 (form) lives in review, doctor, or compile. It is mechanical and could migrate.
- Audit's two halves (alignment + integrity) are grouped by the open-world test; whether they want to stay one verb or split again is open.
- Cadence and who-runs-it are deferred to [[agent-human review boundaries]]'s ladder; this note is about *what each faculty judges*, not *when or by whom*.

## Provenance

Crystallised 2026-06-25 from a conversation tracing what `review.md` / `audit.md` were intimated to be across the specs. Trigger: noticing `review` carries four altitudes (form / structure / consistency / strategic fit) and `audit` was straddling machine-health (now doctor's) and epistemic-integrity. Recorded as a thinking aid for the eventual verb ADRs, not a directive.
