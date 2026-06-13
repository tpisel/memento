---
title: Review verb and agent-assisted maintenance
status: accepted
mode: read-only
date: 2026-06-11
tags: [memento, review, maintenance, roadmap]
summary: A `review` verb is accepted as v4 roadmap work, distinct from `compile` (which must stay deterministic and hook-safe). Mechanical review reports duplicate headings, malformed frontmatter, stale summaries, count-1 tags, and broken wikilinks. Agent-assisted review uses the caller's reasoning to propose better summaries, identify obsolete notes, and suggest decomposition — memento itself embeds no model credentials.
---

# ADR-0006 - Review verb and agent-assisted maintenance

## Context

Memento should tolerate sparse and imperfect markdown, but a durable memory vault still benefits from periodic maintenance. Some issues are mechanical, such as duplicate headings or malformed frontmatter. Others benefit from agent judgment, such as improving summaries or suggesting when a large spec should be split.

This maintenance should not be part of `compile`. Compile must remain deterministic, pure, fast, and hook-safe.

## Decision

A future `review` verb is accepted as v4 roadmap work.

The mechanical form of `review` may:

- report duplicate headings and their generated anchors;
- report malformed or inconsistent frontmatter;
- flag missing or stale summaries;
- flag count-1 tags as possible typos;
- report broken wikilinks or suspicious link targets;
- standardize formatting where the operation is deterministic and low-risk.

An agent-assisted form may use the calling agent's reasoning to:

- propose better summaries;
- identify duplicated or obsolete notes;
- suggest spec decomposition;
- suggest typed links where repeated retrieval misses show a real dependency.

Agent-assisted review borrows agent power from the caller. The memento binary itself does not embed model credentials or perform network summarization during compile.

## Consequences

- Maintenance has a home without bloating v0.
- Compile stays suitable for pre-commit hooks.
- Review warnings can guide human curation without making sparse adoption fail.
- Agent-powered maintenance remains optional and explicit.

