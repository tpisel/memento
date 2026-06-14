---
title: "@N-prefixed numeric brief references"
status: accepted
supersedes: "[[Architecture decision record/adr-0011-numeric-brief-references|ADR-0011]]; partial: Decision bullets on numeric refs and wikilink suffix; Determinism paragraph on hash header location."
mode: read-only
date: 2026-06-14
tags:
  - memento
  - brief
  - read
  - agents
summary: "Numeric manifest references use an explicit `@N` form in both CLI reads and brief wikilink suffixes. Bare digit arguments stay on the normal key/path path. The brief manifest hash lives in frontmatter so Obsidian still recognizes line-1 frontmatter."
---

# ADR-0016 — @N-prefixed numeric brief references

## Decision

ADR-0011 remains accepted, but three details are refined before implementation.

First, CLI numeric references use an explicit **`@N` prefix**. `memento read @5` reads manifest entry 5. Bare arguments, including all-digit strings such as `5`, `5.md`, or a vault file literally named `5`, continue through the ordinary key/path resolution path.

This removes the all-digits overload in ADR-0011. A positional manifest reference is now visibly a positional manifest reference, while source-compatible path strings remain path strings.

Second, brief wikilink suffixes use the same **`@N` notation with no space between `@` and the digit**.

For bare wikilinks, the brief synthesizes display text from the resolved target so Obsidian keeps the link target intact:

| Source form | Brief form |
|---|---|
| <code>&#91;&#91;target&#93;&#93;</code> | <code>&#91;&#91;target&#124;target @N&#93;&#93;</code> |
| <code>&#91;&#91;target&#124;display&#93;&#93;</code> | <code>&#91;&#91;target&#124;display @N&#93;&#93;</code> |

Anchored wikilinks and unresolved wikilinks remain unchanged. The renderer must not convert <code>&#91;&#91;target#Heading&#93;&#93;</code> or an unresolved <code>&#91;&#91;unknown&#93;&#93;</code> into a suffixed display form because doing so would either obscure the anchored read path or imply a manifest entry that does not exist.

Third, the brief manifest hash moves into the brief's frontmatter instead of a leading HTML comment. The brief still carries a manifest hash, but `---` returns to line 1 so Obsidian recognizes the frontmatter guard from ADR-0008.

The frontmatter shape is:

```yaml
---
manifest: sha256:abc1234
mode: read-only
---
```

## Context

Dogfooding surfaced three concrete mismatches in ADR-0011's original design.

Bare numeric CLI arguments were ambiguous. `read 5` was easy after `brief`, but it forced the parser to treat all-digit strings specially forever. That conflicts with memento's path-first addressing model: a vault can contain files whose names are numeric or begin with numeric segments, and the durable key should not have surprising exceptions.

The wikilink suffix shape in ADR-0011 was also wrong for Obsidian. Rendering <code>&#91;&#91;beta&#93;&#93;</code> as <code>&#91;&#91;beta @ 2&#93;&#93;</code> changes the target to a literal note named `beta @ 2`; it does not add display text. The correct Obsidian form is <code>&#91;&#91;beta&#124;beta @2&#93;&#93;</code>, with the durable target on the left of the pipe and the display text on the right.

Finally, ADR-0011's line-1 HTML comment for the manifest hash broke one of ADR-0008's editing guards. Obsidian only recognizes frontmatter when the opening `---` is the first line. A generated file can carry both the hash and the `mode: read-only` guard, but the hash must live inside frontmatter.

## Consequences

- CLI examples and implementation use `memento read @N`, not `memento read N`.
- `read` does not need an all-digits special case. The `@` sigil selects manifest-index resolution; every other argument remains key/path resolution.
- Brief output gives agents one notation to remember: entry markers and wikilink display suffixes both use `@N`.
- Obsidian wikilinks remain clickable because the target portion is preserved.
- The generated brief keeps ADR-0008's line-1 frontmatter guard and still exposes the manifest hash for stale-brief detection.
- ADR-0011's broader model is unchanged: numbers remain ephemeral, derived from manifest ordering, valid only across a stable `brief -> read` cycle, and never durable identifiers.

## Supersedes

This ADR partially supersedes ADR-0011:

- Decision bullets covering all-digit `read <N>` and wikilink suffix rendering are replaced by the `@N` forms above.
- The Determinism paragraph that places the manifest hash in a line-1 HTML comment is replaced by the frontmatter `manifest:` field above.
