---
title: "Read-time link surface — stderr, role-flattened, section-scoped"
status: accepted
mode: read-only
date: 2026-06-15
tags:
  - memento
  - read
  - links
  - manifest
summary: "`memento read` surfaces in- and out-links of the read content on stderr alongside the existing `binding:` line. Stdout remains the raw body — no annotation, no metadata, no @N substitution. Links are role-flattened (`inlinks:`, `outlinks:`, `transcludes:`, `transcluded-by:`), empty roles omitted, each entry suffixed with `@N`. Section reads (`read key#heading`) derive surfaces from the section excerpt, not the whole file. Typed edges beyond `wikilink`/`embed` come with the work that introduces them; this ADR ships the surface for the two edge types currently in the graph."
---

# ADR-0021 — Read-time link surface: stderr, role-flattened, section-scoped

## Decision

`memento read` gains a structured link surface, emitted to stderr alongside the existing `binding: ratified|unratified` line.

Concretely:

- **Channel: stderr.** Stdout remains the raw markdown body, unannotated. The body is what was authored; nothing is injected, rewritten, or appended. A consumer running `memento read X > saved.md` gets bytes-equivalent to the source file's body (modulo standard stdout buffering).
- **Format: role-flattened sections, one per edge role, omitted when empty.** Each role renders as one line: `<role>: <key> @N, <key> @N, ...`. The roles available at v2:

  | Role | Source | Direction |
  |---|---|---|
  | `inlinks` | `wikilink`-typed edges pointing at this content | incoming |
  | `outlinks` | `wikilink`-typed edges leaving this content | outgoing |
  | `transcludes` | `embed`-typed edges leaving this content (`![[target]]`) | outgoing |
  | `transcluded-by` | `embed`-typed edges pointing at this content | incoming |

  Empty roles are omitted entirely; a doc with no inlinks and no embeds renders only `outlinks:` (and only if it has any).

- **`@N` index suffix on every entry.** Each resolved target's brief-render numeric index (per ADR-0016) is appended for cheap follow-through. `outlinks: target @4, other @7`. Unresolved targets render with their raw wikilink target and no `@N` suffix.
- **Section reads derive the surface from the section excerpt.** For `memento read key#heading`, in/out links are computed against the rendered section's text and its anchor, not the whole file. A whole-file read uses the file's complete link graph as today.
- **Stderr emission order is fixed:** `binding:` line first (existing), then link roles in the table order above. This is a stable contract — consumers may parse it.
- **Typed edges beyond `wikilink` and `embed` are deferred.** Spec §7's typed-link overlay (`depends-on`, `see-also`, `supersedes`, `embeds`) names four types, but the compile-time extractor today emits only two: `wikilink` (default) and `embed` (`![[ ]]`). This ADR's surface ships those. New roles (`supersedes:`, `superseded-by:`, etc.) come with the ADR/bead that introduces the corresponding typed edge into the link graph — likely the ADR-convention work that wires frontmatter `supersedes:` to a `supersedes`-typed edge.

## Context

The manifest already carries an out/in link graph per ADR-0009 / spec §4 — outlinks resolved to manifest keys plus typed (`wikilink` or `embed`), inlinks inverted at compile time. Spec §7 names "links on `read` for navigation" as v2 scope. The design choices to pin were the **channel**, the **shape**, and the **scope**.

**Channel.** Stdout vs stderr. Stdout has one job: hand back the raw body for piping, redirecting, and feeding into an LLM context. Adding a metadata footer to stdout poisons the redirect case (`> saved.md` would include a "Links:" footer in the saved file). Stderr is already the established sidechannel — `binding:` ships there per spec §7, and `memento brief` already uses stderr for non-fatal warnings. Extending it for link metadata fits the existing convention and preserves stdout purity. Worth being explicit: the value of stdout purity is partly defensive (an agent that assumes raw bytes does not get burned) and partly compositional (`memento read X | wc -l`, `memento read X > saved.md` work without `2>/dev/null`).

**Shape.** Two candidates were weighed:

- *Direction + type-in-parens:* `inlinks: A (wikilink), B (embed)` / `outlinks: C (wikilink)`. Compact; type info is parenthetical.
- *Role-flattened:* one line per role (in/out × type), empty roles omitted.

Role-flattened was chosen. It makes edge type first-class in the surface — an agent pattern-matching "if I see `transcludes:`, the target's content is *quoted* here, follow it before reading further" is cleaner against role lines than against parenthetical type tags. Empty-role omission keeps the surface terse: a typical leaf note renders only `outlinks:` (and only if it has any), not four blank lines.

**Scope.** A section read (`read key#heading`) should logically surface section-scoped links — the in/outlinks for *this section*, not the whole file. The agent's question is about this section; whole-file links are noise. Two implementation notes:

- *Outlinks* are easy: re-run the link extractor over the rendered section text.
- *Inlinks* require the manifest to retain the `#anchor` portion of resolved outlinks so an inlink edge can be filtered by anchor target. The current manifest stores `target` as the resolved manifest key; anchor preservation is a small additive schema bump (still `schema_version: 1` per the additive-fields convention). If anchor data is unavailable at v2 land time, inlinks degrade to *file-scoped* (whole-file inlinks rendered against a section read), not omitted — explicit rather than silent.

**Outlinks on stderr at all.** Outlinks are visible in the body as wikilink syntax — the agent can scan for them. Surfacing them on stderr was initially questioned. The case for inclusion: the stderr line carries *resolved* targets and `@N` indices, which the body does not, and which the agent needs to call `memento read @N` against. Re-deriving that resolution from a scan of body wikilinks duplicates work the manifest already did at compile time. Stderr therefore ships both directions, symmetric.

## Consequences

- The agent's read protocol becomes: stdout = body, stderr = `binding:` then role-flattened links. Both are documented in orient.
- `memento read X > saved.md` and `memento read X | <pipe>` behave as today — stdout is byte-equivalent to the source body.
- Section-scoped reads return less link noise than whole-file reads, matching the "decomposition at read-time" framing in spec §7.
- Manifest schema may grow an additive `anchor` field on resolved outlinks. Still `schema_version: 1` if additive only; if a non-additive change is required, bump and add an `manifest-schema-unsupported` test case.
- New typed edges (`supersedes`, `see-also`, etc.) become incremental: each adds two roles to the table (incoming + outgoing) when the typed edge enters the graph. The render code's role enumeration is data-driven, not hardcoded.
- Spec §7 "links on `read` for navigation" and spec §15's read-time link navigation deferred item are both resolved.

## Open questions

- **Anchor preservation in manifest outlinks.** Today's manifest may or may not retain the `#anchor` portion of resolved targets. The implementation bead confirms and bumps the schema if needed (additively).
- **Section-inlink fallback policy.** If a v2 implementation cannot do anchor-filtered inlinks on day one, the fallback is whole-file inlinks rendered against the section read. Documented as explicit-not-silent; revisited if it produces noise complaints.
- **Whether `outlinks:` should ever be suppressed when the body already contains the wikilinks.** Lean no — the resolved targets and `@N` indices are the value-add; symmetry with inlinks is cleanest. Revisit if the surface becomes noisy in practice.
- **Cap on entries per role.** A doc that points at 50 others would render a long stderr line. No cap shipped; observe in dogfooding before adding one.

## Supersedes (partial)

- Spec §7 link-on-read framing — refined to pin channel (stderr), shape (role-flattened), and scope (section-aware). The "deferred typed-link overlay" framing in §7 is unchanged.
- Spec §15 "Read-time link navigation surface and typed-link traversal policy" deferred item — closed for the two edge types currently in the graph; the traversal-policy half remains a future-typed-edge concern.

## Related

- [[adr-0016-at-prefixed-numeric-brief-references]] — `@N` numeric references; this ADR uses them as the index suffix in link entries.
- [[adr-0008-memento-brief-projection]] — brief renders wikilink display text with `@N` suffix; this ADR keeps that pattern on stderr metadata.
- Spec §4 / §7 — the manifest's out/in link graph and the read-time consumption framing.
