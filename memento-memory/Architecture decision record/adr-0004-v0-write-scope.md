---
title: V0 write scope
status: accepted
mode: read-only
date: 2026-06-11
tags: [memento, write, mcp]
summary: V0 write supports only creating a new file and appending to an existing one. Section-replace, keyed-upsert, and full mode enforcement are deferred to v2; declared `read-only` files remain socially read-only until enforcement lands. The CLI must not offer operations that imply historical rewriting.
---

# ADR-0004 - V0 write scope

## Context

The specification defines several write modes: `append-only`, `section-replace`, `keyed-upsert`, and `read-only`. Those modes are important because the MCP write surface should eventually enforce memory hygiene mechanically rather than relying on prompt discipline.

However, full write-mode support is not required to validate the first retrieval loop, and implementing all modes at once would put significant complexity before the tool has proven its core usefulness.

## Decision

V0 write support is intentionally narrow:

- create a new memory file;
- append to an existing memory file;
- refuse writes that would overwrite or structurally edit existing content.

Full write-mode enforcement is deferred. `section-replace`, `keyed-upsert`, and richer mode-aware editing are v2 work.

Accepted ADRs and other `read-only` files remain socially read-only until full enforcement exists. The CLI should avoid offering operations that imply historical rewriting.

## Consequences

- V0 can expose and exercise the future MCP-shaped write surface without taking on all editing semantics immediately.
- The first implementation prioritizes retrieval correctness and deterministic manifests.
- Agent write-back remains conservative during the period when memory-writing habits are still being evaluated.
- The eventual MCP write API still has a clear destination: mechanically enforce declared modes before mutating files.

