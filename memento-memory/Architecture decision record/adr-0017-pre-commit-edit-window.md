---
title: Pre-commit edit window for new documents
status: accepted
mode: read-only
date: 2026-06-14
tags:
  - memento
  - write
  - mode
  - lifecycle
summary: "A document that has not yet been committed to git is in its 'edit window': the write gate accepts full-file rewrites regardless of declared `mode:`. Once the document is first committed, the declared mode binds for all subsequent writes. Ratification event = first commit, not session boundary. Rule applies uniformly across `append-only`, `living`, and `read-only`."
---

# ADR-0017 — Pre-commit edit window for new documents

## Decision

A markdown note in the memento vault that has not yet been committed to git is in its **edit window**. While in the edit window, the write gate accepts full-file rewrites regardless of the document's declared `mode:`. The edit window closes the moment the file lands in its first git commit; thereafter the declared mode binds for all subsequent writes.

- **Ratification event = first commit.** A file's presence in the repository's tracked tree flips its binding state from *unratified* (edit-window open) to *ratified* (mode enforced). Cheapest predicate: `git ls-files --error-unmatch <path>` returns 0 iff tracked.
- **Rule applies uniformly across all modes.** `append-only`, `living`, and `read-only` all use the same predicate. A freshly-created note declared `mode: read-only` is freely rewritable until first commit; thereafter the tool refuses writes.
- **The write gate is the enforcement point.** Read operations are unaffected; only the write path's mode-refusal logic gains the edit-window check.

## Context

Protection modes exist to constrain *cross-session* agent behaviour against *settled* artifacts. They do not exist to constrain in-session co-composition between a human and an agent who are still drafting. The current implementation of ADR-0015 (write-mode taxonomy) collapses these two situations: `append-only` binds the moment a file exists, which forces an addendum-instead-of-edit pattern even when the file is minutes old and unseen by anyone else.

The concrete trigger was an OKF interop discussion on 2026-06-14. An append-only note had been alive for under an hour, and a small framing revision required appending an addendum rather than revising the prior text — friction with no safety benefit, because there was no settled artifact to protect.

Two candidates for the ratification event were weighed:

1. **Session boundary** (the original framing). Rejected. memento is stateless across CLI invocations; "session" is the calling agent's concept, not memento's, and would require either an agent-supplied session ID, frontmatter `created_in:`, or `.memento/in-flight.json` keyed by path. All have the same problem: detecting "session end" is itself heuristic. Adds infrastructure for a benefit a sharper rule already delivers.

2. **First commit.** Chosen. Zero new infrastructure. Inspectable — anyone with the repo can determine a file's binding state without out-of-band data. Aligns directly with spec §4 ("diffable = auditable") and §8 ("autonomous write with asynchronous review via git diff"): the commit is *already* the moment the doc enters review. The pre-commit hook fits cleanly — compile runs, manifest updates, binding state flips, all in the same atomic event.

The commit-bounded rule is *slightly* more conservative than the session-bounded one. A session that commits early, then revises, must revise through the declared mode after the first commit. Deliberate trade: the commit is the moment the doc enters the external review surface, and post-review revisions should respect the declared mode regardless of which session is making them.

### Why apply uniformly to `read-only`

The natural concern is whether `read-only` should be the exception — bind immediately on file creation, since the whole point of `read-only` is "no rewrites through the tool, ever, after settlement." Decision is to apply the edit-window rule uniformly anyway:

- The settlement event for a `read-only` doc is also normally the commit. An ADR transitions to "accepted, locked" when the human commits the accepted-status version. Before commit, it is a draft, and drafts iterate.
- A user who creates a `read-only` doc and commits it before completing the draft has a small but real footgun. Mitigation is convention (draft under `mode: living`, change to `mode: read-only` on acceptance), not a special-case in the gate.
- Uniformity keeps the rule teachable: one predicate, one binding event, no per-mode arcana.

If the footgun produces real incidents, revisit with `read-only` as a special case (see Open questions).

## Consequences

- The write gate gains one predicate check: `is_ratified(path)` returns true iff the file is tracked in git. Implementation is a single git invocation, cacheable at vault open.
- The agent-facing read response should surface binding state, so the agent can reason about whether its next write is gate-constrained. Specific surface is implementation detail.
- Default mode behaviour from ADR-0015 is unchanged for committed docs. A doc without an explicit `mode:` is `append-only` once committed and freely writable while in the edit window.
- ADR drafts become freely iterable through the tool until the human commits the accepted version. This matches the natural ADR lifecycle (draft, revise, accept, lock) without requiring a separate drafting-only mode.
- The rule encourages "stay uncommitted while iterating, commit when settled" — which is already the natural workflow. Edit-window state is therefore aligned with normal git hygiene, not a new discipline.
- Vaults that are not in a git working tree cannot evaluate the predicate. See Open questions.

## Open questions

- **Non-git vaults.** memento does not currently mandate git, though the spec heavily assumes it. If a vault is not a git working tree, `is_ratified` cannot be evaluated. Working assumption: conservative fallback ("always ratified", modes always bind). No test scenario forces a decision yet.
- **`read-only` as a special case.** If evidence emerges that the uniform rule lets `read-only` docs be modified in ways that subvert the supersede-don't-edit discipline (spec §12), revisit. Likely shape of the fix: `read-only` binds immediately on first write, all other modes follow the edit-window rule.
- **Multi-commit drafting sessions.** A doc committed during a multi-commit session (e.g., as part of incremental progress) loses edit-window protection on that first commit, even though human composition is not yet complete. The session-bounded framing would have handled this; the commit-bounded rule does not. Trigger for revisit: observed friction from agents being unable to revise their own recently-committed drafts within the same session.
- **Surfacing edit-window state in the brief.** Today the agent learns binding state only through `read`. Whether the brief or manifest should also expose it (so a session-start scan can plan) is deferred — read-time exposure is sufficient until evidence of need.

## Supersedes (partial)

- ADR-0015 §Decision 2 ("Default mode is `append-only`"): unchanged in principle, but the default mode only binds after first commit.
- Spec §8 ("autonomous write with asynchronous review via git diff"): this ADR makes the "commit as review boundary" claim load-bearing for mode enforcement, not only for human review.

## Related

- `Configurability exploration.md`: per-mode override, commit-count boundary, and other knobs that could relax or tighten the edit-window rule if evidence demands.
