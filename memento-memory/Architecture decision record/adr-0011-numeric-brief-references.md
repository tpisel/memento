---
title: Numeric brief references — ephemeral CLI projection over manifest entries
status: accepted
superseded_by: "[[Architecture decision record/adr-0016-at-prefixed-numeric-brief-references|ADR-0016]]; partial: Decision bullets on numeric refs and wikilink suffix; Determinism paragraph on hash header location."
mode: read-only
date: 2026-06-13
tags:
  - memento
  - brief
  - read
  - agents
summary: "The brief gains a render-time numeric index over top-level entries so `read N` is the short follow-up after `brief`. Numbers exist only in CLI/brief output — never in source, wikilinks, commit messages, or beads tasks. `read N` resolves against `.memento/manifest.json` only, never falls back to a filesystem walk; if the manifest is stale (rename/delete) the call is a hard error that names the fix verbatim. Folder structure surfaces as H2 sections; section addressing stays slug-based."
---

# ADR-0011 — Numeric brief references: ephemeral CLI projection over manifest entries

## Decision

The brief gains a **render-time numeric index** over top-level entries. The agent's follow-up after `memento brief` becomes `memento read 4` instead of `memento read "Architecture decision record/adr-0003-v0-retrieval-and-indexing-semantics.md"`.

Numbers are CLI/brief-output only. They do not appear in source files, wikilinks, commit messages, beads tasks, or any cross-vault reference. They are an *ergonomic projection*, not an identifier.

Concretely:

- Brief renders entries with a numeric prefix (`1`, `2`, ...) running continuously across the document.
- Brief gains folder-as-H2 grouping. Each directory under the vault becomes an H2 section; entries within sort lexicographically by filename. Numbering runs continuously across groups so `read N` is unambiguous; folders are presentational.
- Brief carries a manifest content-hash header comment on line 1: `<!-- manifest: sha256:abc1234 -->`.
- Wikilinks in summaries re-render with their index suffix: `[[spec]]` → `[[spec @ 3]]`. Source files are untouched.
- `memento read <N>` accepts an all-digit argument, resolves against the current `.memento/manifest.json` using the same ordering function as the brief renderer, and reads the resolved file. Pure read; never mutates; never falls back to a filesystem walk.
- Section addressing stays slug-based: `memento read <key>#<heading-slug>`. No two-level numeric refs.

## Context

V0 `read` works against full paths (ADR-0003). Paths in a real vault are long — `Architecture decision record/adr-0003-v0-retrieval-and-indexing-semantics.md` for a single ADR. `brief → read` is the dominant agent retrieval loop, and that loop currently requires the agent to type or copy-paste the path. The friction is small per call but compounds across a retrieval-heavy session.

A frontmatter stable-`id:` system was rejected in spec.md §5 as authoring friction that diverges from how wikilinks actually address content. Numeric refs sidestep that rejection because they live at a different layer: not stored, not authored, regenerated on each brief render, scoped to one CLI cycle. The durable identifier remains the path; the number is a view.

Two adjacent ideas were considered and rejected:

- **`brief --index` (titles only, no summaries).** Rejected: the brief's design (ADR-0008, `what makes a good summary.md`) leans hard on summaries carrying the load-bearing fact. An index-only mode invites decide-by-title and degrades retrieval quality. If brief output is too noisy, tighten summaries — do not offer a shallower variant.
- **Two-level numeric refs for section reads (`read 4 2`).** Rejected: section slugs are greppable from rendered brief output, round-trip with wikilinks (`[[adr-0003#Ignore Semantics]]`), and self-document in shell history. Numeric pairs save a few characters per call but cannot be read back later and conflict with future positional CLI arguments.

## Determinism and the staleness contract

`read N` resolves against `manifest.json`, not against a fresh filesystem walk. This is forced by correctness: if `read` recomputed ordering from the filesystem on every call, numbering could shift between `brief` and `read` (a new file added, a rename, a delete) and silently return a different entry than the agent saw. Using the manifest as the resolver source means numbering is stable for the full window between commits — and because `compile` is hook-gated (ADR-0005), `manifest.json` only mutates at commit time, which matches the agent's likely session window.

The brief itself renders from `manifest.json`, not from a fresh scan. Brief and resolver share the source of truth so they cannot disagree.

The ordering function is total, pure, and stable: lexicographic by (folder path, filename) with a fixed folder collation order. Inserting a new file shifts numbers for subsequent entries in the same group; renames may shift across groups. This is irreducible — without durable IDs (rejected), any positional index is sensitive to inserts. The contract is honest about that.

Staleness handling:

- **File missing on disk** (renamed or deleted since last compile) → hard error:
  ```
  entry 5's file `Architecture decision record/adr-0001-vault-naming.md` no longer exists.
  manifest is stale; run: memento compile && memento brief
  note: entry numbers will likely shift after compile.
  ```
- **Manifest hash drift** (compile happened since last brief render, file still present) → warning to stderr, read succeeds against current manifest's entry N:
  ```
  warn: manifest changed since last brief, numbers may not match your view — re-run memento brief.
  ```
  The brief's most-recent render hash is read from `_memento/brief.md`'s header comment.
- **No auto-compile-and-retry.** `read` is pure; recompile is the agent's call because they are the one whose other in-flight references may also shift. Auto-recompile would silently invalidate the agent's context.

**Contract, one line.** Numeric refs are valid only across one `brief → read` cycle on a stable manifest. Any compile invalidates them; that's by design and surfaced explicitly.

## Consequences

- The `brief → read` loop drops from a long path argument to one or two characters. Retrieval-heavy sessions feel materially lighter.
- The brief gains visible directory structure (H2 per folder), which doubles as a navigation aid even when numbers aren't used.
- `read`'s capability boundary stays pure-read. Recompile timing remains an explicit agent action.
- spec.md §5 (no durable IDs) is unaffected. Paths remain the canonical key; numbers are a render-time view.
- The manifest schema is unchanged. Numbering is derived at projection time.
- ADR-0008 extends rather than is rewritten — the brief format adds H2 folder sections, a hash header line, numeric prefixes, and wikilink suffixes; the rest of the rendering is unchanged.
- A small new failure mode (post-rename `read N` errors with stale-manifest message) replaces what would otherwise be silent-wrong-file. The failure mode is shared with path-based `read` of a renamed file — numeric refs do not extend it, they inherit it.

## Explicit non-features

- No `brief --index` mode.
- No two-level numeric refs (`read 4 2`).
- No rename detection or content-hash file resolution on stale reads.
- No persistence of numbering outside the rendered `_memento/brief.md`.
- No use of numeric refs in source files, wikilinks, commit messages, or beads tasks. Cross-vault references continue to use the path or wikilink form.

## Open questions

- Whether brief output should render the per-entry size marker (ADR-0008, bead memento-2nb.30) on the same line as the numeric prefix or below — a layout call to make once both ship.
- Whether the brief's manifest hash should also appear as a quotable footer line (so an agent can report "read against brief @ abc1234"). Deferred; not load-bearing for v0.
