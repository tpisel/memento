---
title: V0 retrieval and indexing semantics
status: accepted
mode: read-only
date: 2026-06-11
tags: [memento, compile, read, markdown, ignore]
summary: V0 scope is deterministic indexing and selective reads — one-vault discovery via marker, deterministic `manifest.json`, `read <key>` for whole files, `read <key>#<heading>` for section reads with GitHub-style slug anchors. `.mementoignore` supports a small `.gitignore`-like subset (literals, globs, `**`); negation, nested ignores, and full git-attribute compatibility are deliberately out of scope.
---

# ADR-0003 - V0 retrieval and indexing semantics

## Context

The first useful loop is retrieval, not authoring automation. Memento should prove that a human-authored markdown vault can be indexed into a compact manifest, scanned at task start, and read selectively by an agent.

Adoption must work on sparse markdown. Existing notes may have no frontmatter, inconsistent tags, and uneven heading structure. The v0 implementation should therefore extract useful structure without requiring users to retrofit their vault before seeing value.

## Decision

V0 focuses on deterministic indexing and selective reads:

- discover exactly one vault via `.memento/`;
- compile a deterministic manifest at `.memento/manifest.json`;
- support `compile --print` for testing and inspection;
- parse markdown and frontmatter using `goldmark`, with memento-specific scanners and editors around it;
- extract title, summary fallback, tags, H2/H3 headings, write mode, updated metadata, summary staleness, and links where available;
- support `read <key>` for whole-file reads;
- support `read <key>#<heading>` for section reads.

Heading anchors use GitHub-style slug normalization. Duplicate slugs are disambiguated by appending `-1`, `-2`, and so on. Duplicate headings are not an error because deterministic anchors make them addressable, but review tooling may warn about them later.

Manifest output must be deterministic, including path form, entry ordering, JSON key ordering, timestamp formatting, tag ordering, and heading/link ordering. The manifest is a committed artifact, so noisy diffs are a correctness problem.

## Ignore Semantics

V0 supports a small `.gitignore-like` language in the memory root's `.mementoignore`:

- `#` starts a comment;
- `\#` represents a literal leading hash;
- `foo.md` matches a file named `foo.md` wherever it appears;
- `/foo.md` matches a file named `foo.md` only at the memory root;
- `foo/` ignores directories named `foo` recursively wherever they appear;
- `/foo/` ignores a directory named `foo` only at the memory root;
- `*` has normal glob behavior within a path segment;
- `**` matches recursively across path segments.

V0 deliberately does not support:

- negation with `!`;
- nested ignore files;
- symlink traversal semantics beyond the default safe filesystem walk;
- include directives;
- git attributes or full gitignore compatibility.

## Consequences

- The v0 product can be useful before write automation or MCP exists.
- The implementation can use robust markdown parsing without pretending memento is a general markdown formatter.
- `.mementoignore` is familiar enough to learn quickly, but small enough to specify and test completely.
- GitHub-style section anchors make links predictable for humans, agents, and external systems such as beads.

