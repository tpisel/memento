---
title: "_memento/ — mixed-audience tool namespace"
status: accepted
mode: read-only
date: 2026-06-13
tags: [memento, namespace, vault, agents, write, review]
summary: "Establish `<vault>/_memento/` as the namespace for tool-relevant human-readable artifacts — both memento-generated (brief, future review outputs) and user-authored (writing guides, review prompts, audit instructions). Ownership is file-level, not folder-level: `.gitignore` distinguishes generated from versioned files, `mode:` and filename conventions handle the rest. Pinning specific tool-read filenames is deferred to ADR-0010."
---

# ADR-0009 — _memento/: mixed-audience tool namespace

## Decision

The vault carries two parallel tool namespaces:

- **`<vault>/.memento/`** — the *machine* namespace. Hidden from Obsidian by design. Holds `manifest.json`, `config.toml`, and any future binary/structured tool state. Not browsed by humans.
- **`<vault>/_memento/`** — the *human-readable tool* namespace. Visible in Obsidian (the underscore sorts it to the top of the file tree and signals "tool-owned"). Holds both memento-generated artifacts (e.g., `brief.md` per ADR-0008) and user-authored content that the tool reads at well-known paths.

**Ownership inside `_memento/` is file-level, not folder-level.** Three mechanisms together do the work:

1. **`.gitignore` granularity.** The vault `.gitignore` block lists specific generated files (`_memento/brief.md`, later `_memento/reviews/*.json`). Files not listed are versioned by default — i.e., user-authored.
2. **`mode:` frontmatter.** User-authored guides carry `mode: read-only` like any other human-curated note; agents read them, the tool refuses to mutate them. Generated files also carry `mode: read-only` plus a DO-NOT-EDIT banner (per ADR-0008).
3. **Filename conventions.** Tools look for specific paths at specific moments — `_memento/writing.md` read by `memento write`, `_memento/review.md` read by `memento review`, `_memento/audit.md` read by future audit, etc. The filename is the contract; the canonical list is pinned by ADR-0010 when those verbs land.

## Context

The current design has manifest.json under `.memento/` (machine-readable, hidden) and was about to place `_brief.md` at vault root for Obsidian visibility. As we extended the design to anticipate user-authored writing guides, review prompts, and audit instructions, several things became clear:

- Tool-relevant human-readable artifacts will accrue, not stay at one file. They need a home.
- Strict "memento owns it / humans own it" folder boundaries collapse the moment users want to encode discipline that the tool consumes. A user's writing guide is *about* tool behavior even though they author it.
- A single folder for tool-relevant artifacts lets agents onboard to a vault by reading one place — they learn what's there *and* how this vault expects to be edited.
- Different vaults will carry different conventions. The tool should ship sensible defaults and let users override per-vault, without prescribing structure on the content namespace itself.

Two alternatives were considered:

- **Subdivide `_memento/` into `generated/` and `config/`.** Rejected as premature: it imposes a structure before we know the cardinality of either bucket, and the `.gitignore` mechanism already gives clean file-level discrimination without an extra layer.
- **Spread tool-relevant files across the vault root.** Rejected: it pollutes the user content namespace and gives no obvious place for an agent to look for vault-specific conventions.

The content namespace itself (everything outside `.memento/` and `_memento/`) remains deliberately non-prescriptive. Memento does not require ADRs, a particular folder structure, or any specific document type — see ADR-0003. Opinionated `init --template=` starting points may arrive later but will remain opt-in.

## Consequences

- The vault's tool-relevant surface lives in one place. Agents onboarded to a new vault read `memento brief` and then scan `_memento/` for vault-specific conventions before authoring or reviewing.
- The brief renderer (ADR-0008) gains a small footer listing tool-consumed files present in `_memento/`, so an agent can see "this vault has writing rules" without separately probing for files.
- A vault's conventions travel with the vault: cloning the repo gets the writing/review/audit guides; the manifest and brief regenerate locally.
- `compile` walks `_memento/` like any other content directory for indexing user-authored guides into the manifest (so an agent can find them by name), while ignoring the specific files listed in `.mementoignore` (just `brief.md` for now).
- Future deferred work:
  - **ADR-0010** pins specific filenames (`writing.md`, `review.md`, `audit.md`, ...) and their precedence rules against memento-shipped defaults. Lands when the corresponding verbs are designed.
  - **`init --template=`** opt-in vault starters. Out of scope here.

## Open questions

- Should `compile` index files under `_memento/` into the manifest by default, or treat the folder as out-of-namespace for retrieval? Current lean: index them, because they describe vault conventions an agent may want to surface. Worth revisiting once concrete tool-read files exist.
- Whether memento ships any default content in `_memento/` on greenfield init beyond a convention README. Likely just the README at v0; templates later.
