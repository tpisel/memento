---
title: What makes a good summary
status: draft
mode: append-only
date: 2026-06-13
tags: [memento, summaries, writing, draft]
summary: A summary is read by an agent scanning the brief to decide what to open next. Lead with the decision or the load-bearing fact — not the question, the context, or the motivation. If reading the summary tells you the answer, the brief is doing its job; if it only tells you a file exists on the topic, the brief is just an index.
---

# What makes a good summary

This is a working draft. The discipline below was extracted from reviewing the v0 manifest and noticing how many summaries described what each document was *about* rather than what each document *said*. It will be refined into an operational guide later, possibly enforced by the future `review` verb (ADR-0006).

## The rule

**Lead with the answer, not the question.**

An agent does not read summaries to learn that a topic exists. It reads summaries to decide whether the body of the document will help with the task at hand. If the summary tells you *what was decided*, the agent can often act without opening the file at all. If the summary only tells you *what question is discussed inside*, the agent has to open every plausibly-relevant file to find out.

Same constraint for human readers in Obsidian: a wall of "this document discusses X" lines forces every retrieval into a hover-and-click. A wall of "the decision was Y" lines lets the human read the page like a digest.

## Concrete contrast

**Anti-pattern** (describes the question):

> The specification defines several write modes: append-only, section-replace, keyed-upsert, and read-only. Those modes are important because the MCP write surface should eventually enforce memory hygiene mechanically rather than relying on prompt discipline.

You finish reading and you still don't know what was decided about write modes. You have to open the file.

**Better** (leads with the decision):

> V0 write supports only creating a new file and appending to an existing one. Section-replace, keyed-upsert, and full mode enforcement are deferred to v2; declared `read-only` files remain socially read-only until enforcement lands.

You finish reading and you know the v0 surface, the deferred work, and the social-vs-mechanical distinction — without opening the file. The body is now there for *justification*, not *disclosure*.

## Working heuristics

- If the first sentence of the summary could be the first sentence of a "## Context" section, it is probably wrong. Move it down; lead with what the Decision section says.
- A summary that names a specific path, flag, version, or scope boundary almost always beats one that names a category.
- "Tolerates X" / "supports Y" / "is integrated with Z" beats "considers" / "discusses" / "addresses."
- Rejected alternatives belong in the body, not the summary. The summary should answer "what did we do?", not "what did we consider?"
- Length is not the issue. A three-sentence summary that leads with the decision beats a one-sentence summary that leads with the question.

## What this is not (yet)

- Not a tool-enforced contract. Today this is a content-discipline guide for humans authoring frontmatter `summary:` fields.
- Not a style guide. It does not prescribe sentence patterns or banned words.
- Not the final form. Once the `review` verb lands (ADR-0006), parts of this may become mechanical checks (e.g., "summary starts with the same paragraph as the first H2 section that is also titled 'Context'"). Until then, it is a checklist for human review and for agent-assisted summary improvement.
