---
title: Note states
status: reference
mode: living
date: 2026-06-17
tags:
  - memento
  - states
  - review
  - staleness
  - write
  - mode
  - frontmatter
  - conventions
summary: The four-dimensional state space every memento note inhabits — exists × ratified × declared_mode × summary_state — with the cells named, the legal-vs-degenerate distinctions called out, and the projections used by write enforcement, staleness signalling, and the future review verb. Reference for review/staleness/write design conversations.
---

# Note states

This note names the state space a memento note inhabits at any moment. It is reference material — not a contract, not policy. Its purpose is to give review-verb design, staleness-ledger queries, and write-mode reasoning a shared vocabulary for the cells they operate on.

The shape is one paragraph of ADR commitments stacked. Each axis was introduced by a different ADR, and each consumer (write enforcement, brief render, read stderr, future review verb) projects onto a different subset. Naming the full space once, in advance of review's CLI design, prevents the verb from rediscovering it cell-by-cell.

## The four axes

| Axis | Values | Source | Carrier |
|---|---|---|---|
| `exists` | yes / no | filesystem | vault walk |
| `ratified` | yes / no | git working tree vs `HEAD` | `git ls-files --error-unmatch` |
| `declared_mode` | `append-only` / `living` / `read-only` (+ `unparsed` sentinel) | frontmatter `mode:`, default `append-only` | parsed at read/write time |
| `summary_state` | `current` / `stale` / `missing` | ledger `body_sha` × `summary_sha` comparison | `.memento/manifest.json` per-entry |

ADR origins:

- `exists` — spec §4 (the manifest is rebuilt full each compile; presence is the existence of a markdown file under the vault walk).
- `ratified` — [[adr-0017-pre-commit-edit-window]]. The edit-window concept lives entirely on this axis: `ratified = no` *is* the edit window.
- `declared_mode` — [[adr-0015-write-mode-taxonomy]]. Tier 1 frontmatter ([[adr-0014-canonical-frontmatter-vocabulary]]). A fourth value, `unparsed` (`markdown.ModeUnparsed`), is a derived sentinel rather than a declarable mode: when a note's frontmatter fails to parse, the whole block is discarded and the mode resolves to `unparsed`, **not** the `append-only` default. A parse error must never quietly make a note more locked-down — or invert a declared `mode: living` — than its author wrote (memento-o0a). It is held read-only by the gate, surfaced with a `⚠ unparsed` marker in brief, and raised as a `MALFORMED FRONTMATTER` alarm on compile that gates the commit under `MEMENTO_STRICT_COMMIT`.
- `summary_state` — [[adr-0023-summary-staleness-in-ledger]]. The three-state model (current / stale / missing) replaces the prior boolean and moves storage out of frontmatter into the ledger.

Together these give a 2 × 2 × 3 × 3 = 36-cell grid. Most cells are legal; a handful are degenerate or vacuous; only a small projection drives any given consumer.

## Degenerate cells

The grid is regular but a few cells collapse:

- **`exists = no`** removes all other axes. There is no entry to talk about; the only relevant projection is "manifest entries with no on-disk file" — which is `manifest-stale` ([[spec]] §10 error tokens) and is handled at the `@N`-resolution layer, not the per-note state layer. For everything else in this note, assume `exists = yes`.
- **`exists = no × ratified = yes`** is technically possible — a file deleted in the working tree but still in `HEAD` — but the vault walk does not see it. Outside this state space.
- **`exists = yes × ratified = no × declared_mode = read-only`** is legal but unusual: a brand-new file authored with `mode: read-only` declared up-front but not yet committed. The edit-window rule applies normally (writes accepted regardless of mode), and ratification at first commit binds the mode.

The 17 remaining cells (existing notes only) are the working set.

## Projections by consumer

Each memento surface looks at a different slice of the grid. Naming the slices makes consumer-vs-consumer disagreements about "what should this cell do" tractable.

### Write enforcement (per-call gate)

The write verb projects on `ratified × declared_mode` only. `summary_state` and the file's content are irrelevant to the gate.

| ratified | declared_mode | append | overwrite |
|---|---|---|---|
| no | any | accept | accept |
| yes | `append-only` | accept | reject (`mode-rejects-write`) |
| yes | `living` | accept | accept |
| yes | `read-only` | reject | reject (`mode-rejects-write`) |
| yes | `unparsed` | reject | reject (`unparsed_mode`) |

The `unparsed` row fails closed to the same lattice as `read-only`: with no readable declared mode, the safest treatment is to deny every write until the frontmatter is fixed. Repairing the frontmatter is *not* a drive-by mode change (an unparsed baseline has no known prior mode to protect), so that defense allows it and defers to the gate/grant. The `ratified = no` row is the edit window. The whole purpose of [[adr-0017-pre-commit-edit-window]] is to make pre-commit iteration cheap while keeping post-commit binding strict.

### Brief render

Brief projects on `summary_state` only (and on the inline metadata: mode, tags, headings).

- `current` → render the summary text with no marker.
- `stale` → render the summary text with an inline staleness marker; counted in the footer staleness aggregate.
- `missing` → render the first-paragraph fallback (or omit) with an `unsummarised` marker; counted in the footer staleness aggregate.

The brief intentionally does *not* surface `ratified` or `declared_mode` in the staleness counters — they are independent signals.

### Read stderr

Read projects on `ratified × summary_state`. Both emit as separate lines:

- `binding: ratified | unratified` — ratification.
- `summary: current | stale | missing` — summary state.

These are independent and both useful: an agent reading a `ratified | missing` doc knows the doc is locked and yet review-worthy; an agent reading an `unratified | current` doc knows it can edit freely and the summary is up to date.

### Review verb worklist (v4, prospective)

The review verb's mechanical worklist is exactly the union of these cells:

- `summary_state = missing` (any ratification, any mode) — needs a summary.
- `summary_state = stale` (any ratification, any mode) — summary needs refresh, or body needs to revert toward the summary.
- Adjacent signals not in this grid: count-1 tags ([[adr-0006-review-verb-and-agent-assisted-maintenance]]), broken wikilinks (graph-level), malformed frontmatter ([[adr-0014-canonical-frontmatter-vocabulary]] open question on `mode:` validation), duplicate headings ([[adr-0006-review-verb-and-agent-assisted-maintenance]]).

Whether review further partitions the worklist by `ratified × declared_mode` is a CLI-shape question, deferred. Plausible cuts:

- `ratified = no` worklist items are "still in edit window — likely active work" and may rank lower or split.
- `declared_mode = read-only` items in `stale` state are "supersede or amend" candidates, not "rewrite" candidates — different remedy.

The review-verb ADR will pin which cuts ship.

## Adjacent dimensions, deliberately excluded

Several signals look like they belong here but live on other axes:

- **`body_changed_since_last_compile`** — not a state, a transition. The ledger detects it (and turns `current` → `stale` if the summary did not move with the body); the compile-time derivation absorbs the signal.
- **`body_changed_since_last_review`** — a review-verb-relative dimension that doesn't exist today. Would need a per-entry review-acknowledged-hash in the ledger, parallel to `summary_sha`. Open: whether review-verb introduces it.
- **`link_health`** — broken outlinks, unresolved targets. Lives in the manifest's resolved-vs-unresolved edge data, not in this state space. Surfaces in read stderr ([[adr-0021-read-time-link-surface]]) and feeds review.
- **`orient: true`** participation — a frontmatter-declared surface flag, orthogonal to all four axes. A doc can be `orient: true` regardless of mode, ratification, or summary state.
- **`tags`** — count-1 typo detection is review-verb input but lives on the manifest's tag aggregate, not per-note state.

## Open seams

These are the cells or transitions where current behavior is undefined or tentative, and where review-verb work will most likely need to decide:

- **"stale but summary still accurate"** ([[adr-0023-summary-staleness-in-ledger]] open question). The two-hash rule cannot distinguish "summary text drifted from body content" from "body changed in a way the summary still covers." Today's only refresh path is editing the summary, which re-anchors. An explicit `--ack` action is deferred; review-verb may introduce one if usage shows bulk acks-without-edit are common.
- **Non-git ratification fallback.** [[adr-0017-pre-commit-edit-window]] requires `git ls-files`; in a non-git vault the ratification check has no signal. Current behavior is conservatively "always ratified," which over-restricts unratified-only mode in non-git contexts. Untested; not surfaced. Review will want it nailed down before bundling ratification-aware checks.
- **`ratified = yes × declared_mode = read-only × body changed in working tree`** — legal (the human edited in Obsidian, bypassing memento write), but a write through memento would be rejected. Review-verb may want to flag this as "ratified read-only doc modified outside the mode gate" — a clear-cut bypass signal.
- **`exists = no` for an `@N`-referenced entry** (`manifest-stale`). Adjacent state, handled at `@N` resolution today. Review may want to aggregate manifest-stale references across the vault and report them as "outdated cross-references after rename."
- **Multi-vault state model.** Multi-vault is out of scope (spec §15) but when it returns, this grid extends with a vault axis. Review composes across vaults.

## How to use this note

Reach for it when:

- designing the review-verb worklist shape (which cells does the verb iterate, which does it bundle, which does it cut by);
- arguing about a write-enforcement edge case (which cell does this fall into, and is the current rule's behavior the right one);
- extending the manifest schema (does the new field add a fifth axis, or does it project onto an existing one);
- debugging "why didn't `read` show staleness" (check the consumer's projection — read sees `summary_state` per the table above).

Update by appending sections — this is a living reference, not a frozen artifact. Add cells if the grid extends; add seams if observation surfaces them.
