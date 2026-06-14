---
title: OKF-compatible frontmatter conventions
status: accepted
mode: read-only
date: 2026-06-14
tags:
  - memento
  - frontmatter
  - schema
  - okf
  - interop
summary: "Adopt the OKF v0.1 frontmatter vocabulary into memento's Tier 2 (convention, parsed-and-ignored): `type`, `resource`, `timestamp`, `okf_version`. Promote `description:` to Tier 1 as a summary fallback source (after `summary:`, before first-paragraph). Deeper OKF alignment — wikilink-to-markdown-link conversion, concept-ID `.md` stripping, reserving `index.md`/`log.md` — explicitly deferred to a future configurability decision."
---

# ADR-0018 — OKF-compatible frontmatter conventions

## Decision

Adopt the subset of OKF v0.1 alignment that costs nothing today and preserves the Obsidian-aligned default deployment.

### Tier 2 additions (parsed and ignored)

The following OKF v0.1 frontmatter fields are added to ADR-0014's Tier 2 (convention) vocabulary:

| Field | OKF semantics | Memento behavior |
|---|---|---|
| `type` | Short string identifying concept kind (e.g. "BigQuery Table") | Parsed, not acted on |
| `resource` | URI uniquely identifying the underlying asset | Parsed, not acted on |
| `timestamp` | ISO 8601 datetime of last meaningful change | Parsed, not acted on |
| `okf_version` | OKF bundle version declaration | Parsed, not acted on |

Memento producers may emit these; memento consumers ignore them. Vaults authored against OKF v0.1 parse cleanly without warnings.

### Tier 1 promotion: `description:` as a summary fallback

`description:` (an OKF-recommended field) is promoted to Tier 1 as a **summary fallback source**. The summary-resolution order becomes:

1. Frontmatter `summary:` (current memento behaviour, ADR-0014)
2. Frontmatter `description:` (new — OKF-aligned)
3. First-paragraph fallback (spec §9)

`summary:` retains priority. `description:` only contributes when `summary:` is absent. This preserves a single source of truth for memento-native authoring while letting OKF-authored documents surface a sensible manifest summary without producer-side rewriting.

### What this ADR explicitly does not do

The following deeper OKF alignment steps are **not** adopted and remain deferred:

- **Wikilink-to-markdown-link conversion in vault bodies.** Wikilinks remain the default link syntax (preserves Obsidian as the human surface, ADR-0001 / spec §3).
- **Concept-ID `.md` stripping.** Manifest keys retain the `.md`-suffixed vault-relative-path shape (ADR-0007 key stability, spec §5).
- **Reserving `index.md` and `log.md`.** These remain valid concept notes in default deployment.
- **`type:` synthesis on bare markdown.** memento's bare-frontmatter tolerance (ADR-0005, spec §11) does not synthesise `type:` for missing-frontmatter docs.
- **Special handling of conventional section headings.** `# Schema`, `# Examples`, `# Citations` appear in the heading tree like any other heading; no type-aware rendering.

These items are export-shim concerns or dual-mode deployment concerns. They depend on decisions that have not been made; see [[Configurability exploration]] for the open thread.

## Context

OKF v0.1 was announced 2026-06-13 by Google Cloud. Full alignment analysis lives in [[OKF interop and external compatibility]]. This ADR captures the *free subset* of alignment — frontmatter additions that interoperate with OKF producers and consumers without changing memento's existing model, breaking the Obsidian-aligned default, or pre-building infrastructure for unproven use cases.

Three motivations:

- **Cost is zero.** ADR-0014's unknown-field policy already means OKF frontmatter parses without errors. Naming the fields in Tier 2 just makes the alignment explicit and inspectable, and pins the decision so a future reader does not have to re-derive it.
- **Future producer ergonomics.** An author can write memento notes using OKF-aligned frontmatter and have both formats see it. Removes friction for users who care about cross-format portability.
- **Preserves the option to go deeper without coupled work.** If a future ADR commits to dual-mode init or an export shim, the frontmatter vocabulary is already in place — that ADR does not have to bundle vocabulary work with the harder semantic work.

`description:` was the one field worth promoting to Tier 1 rather than leaving in Tier 2. Reason: it has a clear, non-overlapping role with `summary:`. Treating it as a fallback source costs one extra line in the summary-resolution chain and gives OKF-authored docs sensible manifest entries without producer-side rewriting. Promoting any of the other four was considered:

- `type:` to Tier 1 — Rejected for now. No driver for type-aware behaviour yet (brief rendering, manifest filtering). Promote when a concrete consumer needs it.
- `timestamp:` to Tier 1 — Rejected. memento already derives `updated` from filesystem mtime and `summary_hash` semantics (spec §4, §9). Reading a frontmatter timestamp would compete with those mechanisms.
- `resource:` to Tier 1 — Rejected. No memento behavior conditional on a resource URI exists or is planned.
- `okf_version:` to Tier 1 — Rejected. memento does not currently produce OKF bundles; reading the version on input has no behavior to drive.

## Consequences

- ADR-0014's Tier 2 list grows by four fields. Memento documentation of its frontmatter vocabulary now names OKF compatibility explicitly.
- The summary-resolution chain gains one step. Manifest summaries for OKF-authored docs are non-empty when `description:` is present.
- No existing memento note breaks; no Obsidian-default vault changes shape.
- Future export-shim work (memento → OKF) becomes incrementally smaller — more of the field set is already represented on the read side.
- The deferred items above remain open. A dual-mode init feature, if built, picks them up; this ADR does not pre-commit to the shape of that feature.

## Open questions

- **Promotion of `type:` to Tier 1.** Today `type:` is parsed and ignored. A future ADR could give it semantic weight (type-aware brief rendering, type-scoped reads). No driver yet; defer.
- **Validation of OKF-shaped frontmatter on memento input.** Strict OKF conformance requires non-empty `type:` on every non-reserved file. memento does not enforce this — bare-frontmatter docs remain valid in memento, non-conformant in OKF. If a future export shim is built, it must synthesise `type:` rather than reject the input.
- **Whether memento should ever emit `okf_version:`.** Moot today (no export). If an export verb is built, it should emit `okf_version: "0.1"` in the root index, per the OKF spec.

## Related

- ADR-0014 (canonical frontmatter vocabulary): this ADR extends Tier 2 and adds one Tier 1 fallback. ADR-0014's unknown-field policy is the reason this ADR is cheap.
- [[OKF interop and external compatibility]]: full alignment analysis behind the deferred items, including the dual-mode posture.
- [[Configurability exploration]]: dual-mode init thread.
