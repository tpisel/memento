---
title: Canonical frontmatter vocabulary
status: accepted
mode: read-only
date: 2026-06-14
tags:
  - memento
  - frontmatter
  - schema
summary: "Frontmatter fields are split into three tiers: tool-consumed (schema-locked, memento reads and acts on them), convention (memento sees but does not act on; used for human discipline), and reserved-rejected (will not be supported). Unknown fields are ignored — forward-compatible by default. Per-doc ignore via frontmatter is rejected; collection membership remains `.mementoignore`'s job."
---

# ADR-0014 — Canonical frontmatter vocabulary

## Decision

Frontmatter fields are governed by three explicit tiers. Each tier has different stability guarantees and a different relationship to memento's behavior.

### Tier 1: Tool-consumed (schema-locked)

Memento reads these fields and changes behavior in response. Adding a new field to this tier requires an ADR. Removing one is a breaking change.

| Field | Type | Semantics | Introduced |
|---|---|---|---|
| `title` | string | Overrides the H1-derived title for manifest entries | v0 (spec §4) |
| `summary` | string | Primary manifest summary source; overrides `description:` and the first-paragraph fallback | v0 (spec §4, §9) |
| `description` | string | OKF-aligned manifest summary fallback used when `summary:` is absent | v1 (ADR-0018, spec §9) |
| `tags` | list[string] | Manifest tag field and tag-vocabulary input | v0 (spec §4) |
| `mode` | enum | Write-mode declaration; tool enforces against the declared mode | v0 (spec §8) |
| `summary_hash` | string | sha256 of body excluding frontmatter; staleness detection | v0 (spec §9) |
| `orient` | bool | Doc participates in `memento orient` output when `true` | v2 (ADR-0013) |

### Tier 2: Convention (memento ignores)

These fields appear in vault content by user or project convention. Memento parses frontmatter robustly enough not to choke on them, but does not read or act on them. They exist for human discipline.

Named convention fields include `status`, `date`, `id`, `supersedes`, `assignee`, and the OKF v0.1 fields `type`, `resource`, `timestamp`, and `okf_version` (ADR-0018). Project-specific fields may also appear here.

Convention fields do **not** acquire tool behavior silently. Promoting a convention field to Tier 1 requires an ADR.

### Tier 3: Reserved-rejected

Fields memento has explicitly considered and committed not to support. Documented here so the request need not be re-litigated.

- `id` as a durable cross-doc address. Spec §5 rejects stable-id systems; the manifest key is the canonical address. Authors are free to write `id:` in frontmatter for their own bookkeeping (it falls under Tier 2), but memento will never read it as an identity.

### Unknown-field policy

**Memento ignores unknown frontmatter fields.** Adding novel keys does not break the manifest. Memento does not warn on unknown keys; YAML's permissive parse is the v0 contract.

This gives users (and future memento versions) room to add fields without coordination. The price is no spell-checking for tier-1 field names — `mod: append-only` will silently fail to register. Worth the trade.

## Context

Frontmatter is the natural carrier for **per-doc behavior**: how a single document is rendered, written, or surfaced. It is the wrong carrier for **vault-level policy** like collection membership. The two concerns sit on different axes and want different mechanisms.

This came up concretely around an idea to support per-doc ignore via frontmatter (e.g., `ignore: true` to hide a doc from the manifest without touching `.mementoignore`). That option was considered and **rejected**:

- **Centralisation matters.** `.mementoignore` is one place a reader (human or agent) can look to learn what is hidden. Per-doc ignore scatters that policy across N files with no single point of audit.
- **Two mechanisms for one outcome.** A `.mementoignore` glob and a frontmatter flag would compete, requiring ordering rules ("ignore wins" or "frontmatter wins") that solve no real problem.
- **Existing path already works.** If a user wants a doc visible in Obsidian but hidden from the agent, they add the path to `.mementoignore`. The friction is small and the policy stays auditable.

Frontmatter handles **per-doc behavior** (mode, summary override, orient inclusion). The ignore file handles **collection membership**. Different axes, different mechanisms.

A separate motivation: ADRs are accumulating conventional frontmatter that memento does not read — `status`, `date`, `supersedes` — and there has been no explicit statement of whether those fields are seen and ignored, or "should not be there." Tier 2 names that state out loud: present, parsed, deliberately ignored.

## Consequences

- The tool-consumed vocabulary is small (six fields) and tightly defined. New fields cost an ADR.
- Convention fields cost nothing. Authors and projects are free to standardise on whatever frontmatter shape suits them, as long as it parses as YAML.
- Adding a new tool-consumed field is an explicit design act, not a silent feature.
- The unknown-field policy makes memento upgradable: a future memento version can read fields older versions ignored, without breaking the older versions' parsers.
- `.mementoignore` remains the only place a reader looks to learn collection membership. No competing mechanism.

## Open questions

- **Typed surface field.** ADR-0013 introduces `orient: true` as a boolean. If a second auto-load surface appears (e.g., a writing-guide auto-load on first `memento write`), a typed `surface: orient` / `surface: write-trigger` field would generalise cleanly. The migration from boolean to typed string is a one-time rewrite; not worth pre-empting now.
- **Mode value validation at parse time.** Currently a typo in `mode:` would silently fall through to default. A small validator at compile that warns on unknown `mode:` values (without erroring) is a low-cost addition; not part of v0 scope. Track as a future improvement.
- **Tags shape.** Tags are a flat list of strings. Scoped or typed tags (`tags: [domain:retrieval, kind:adr]`) have come up in informal discussion; deferred until a real use case demands it.
