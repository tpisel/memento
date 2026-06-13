---
title: Beads integration posture
status: accepted
mode: read-only
date: 2026-06-11
tags:
  - memento
  - beads
  - retrieval
  - agents
summary: "Memento does not depend on beads, and beads is not required to use memento — they are integrated, not merged. Memento keys and section anchors stay stable and human-readable so beads tasks, commit messages, and humans can hotlink them directly. The substrate boundary holds: task progress in beads, durable semantic knowledge in memento."
---

# ADR-0007 - Beads integration posture

## Context

Memento and beads occupy different memory substrates. Beads holds task state and working memory. Memento holds durable semantic project knowledge. The design depends on agents consulting the memento manifest before starting relevant work, but relying only on agent initiative may be weak in practice.

Task descriptions are a natural bridge: a task can cite the durable context that is likely relevant before an agent begins implementation.

## Decision

Memento does not depend on beads, and beads is not required to use memento.

Memento keys and section anchors should nevertheless be stable and readable enough for beads tasks, commit messages, and human discussion to reference them directly. A beads task may hotlink relevant memento entries or sections in its description.

The bootloader remains responsible for the general rule:

- current work lives in beads;
- durable discoveries live in memento;
- before task work, scan the memento manifest and read relevant entries or sections.

Future integration can make this smoother, but it should remain an integration between two tools rather than a substrate merge.

## Consequences

- Memento remains a standalone durable memory layer.
- Beads can improve retrieval behavior by front-loading likely relevant memory references into tasks.
- The memory/task boundary stays clear: task progress does not leak into durable semantic notes, and durable learnings are not buried in close notes.
- Section anchor stability matters because external task systems may reference memento content directly.

