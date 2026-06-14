# Agent–human coordination failure modes

Backlog/taxonomy of failure states that show up when humans and long-running agents share a queue, a worktree, and a spec. Sort later — collecting raw examples first.

## Taxonomy sketch

- **Mis-speccing** — the spec (bead, prompt, ADR) is invalid at the moment of execution. The agent reads it correctly and is correctly stuck.
- **Mis-dagging** — the spec is locally valid but the dependency/ordering relationship to siblings is wrong. Work upstream or in parallel has invalidated assumptions baked into the spec.
- **Side-channel state** — work landed outside the queue (untracked WIP, manual edits, out-of-band docs) that the queue's specs implicitly depend on or conflict with.
- **Wrapper misattribution** — scaffolding around the agent (loop wrappers, commit hooks, CI) attributes effects (commits, claims, comments) to the wrong unit of work.
- **Stale claim** — agent stops without closing, leaves a bead `IN_PROGRESS`; queue logic skips it forever while a human assumes it's progressing.

## Examples

### 2026-06-14 — bead memento-2nb.37: mis-spec + mis-dag + wrapper misattribution

Single incident, three overlapping failures. Useful concrete case study.

**The spec (bead .37):** "Create ADR-0012 — @N convention for numeric refs, supersedes ADR-0011 details." Blocks beads .38/.39/.40.

**Reality at execution time:**
- `adr-0012-using-memento-guide.md` already existed and was accepted. The 0012 slot was taken. → **mis-spec**: bead is invalid as written.
- ADRs 0013/0014/0015 existed as untracked WIP in the worktree — user-authored ahead of ralph but never committed under a bead. Sibling beads .42/.43/.44 referenced these ADRs ("Per ADR-0013, implement orient") and assumed they were canonical. → **side-channel state** + **mis-dag**: the "design ADR → implementation bead" ordering was broken — implementation beads were in the queue while the ADRs they cite lived only as untracked files.
- The ralph-loop wrapper's commit step (`git add -A` after each iteration) didn't distinguish "this iteration's agent changes" from "pre-existing user WIP." When .37's agent correctly stopped without edits, the wrapper swept up the user's untracked ADRs and the modified `Feature thoughts.md` into a commit titled `memento-2nb.37: ADR-0012 — @N convention…`. Title and content unrelated. → **wrapper misattribution**.
- Bead .37 was never closed (the agent left a comment, not a close). The queue's `--ready` filter skipped it because it stayed `IN_PROGRESS`; ralph happily moved on. → **stale claim**, plus the spec problem was invisible to the operator until review.

**Recovery:** dropped the bogus commit via `git rebase --onto ee7906e^ ee7906e HEAD`, restored the 5 WIP files from the orphaned commit blob, reset .37 to OPEN with a comment explaining the situation, patched ralph-loop to refuse a dirty worktree at start.

**Generalisable lessons:**
- A bead that names a numbered artifact (ADR-N, schema rev N, migration N) is fragile against any parallel work that touches the same numbering space. The bead should resolve "next free N" at execution, not at authoring time — or fail loudly if N is taken rather than executing on a stale slot.
- Loop wrappers that `git add -A` need a clean-worktree precondition. Otherwise WIP-in-flight is silently consumed by whatever bead happens to be next.
- "Agent stopped without changes" is not the same as "nothing happened." The wrapper has to check whether the bead state advanced (closed vs. commented vs. still-claimed) before deciding what to commit and whether to proceed.

## Open notes / not-yet-examples

- Bead reshape on conflict — what's the right workflow when an agent finds its spec invalid? Comment + leave open vs. block + raise to human vs. auto-reshape.
- DAG drift from ADR mutation — when an ADR is written informally and only later "promoted," the bead DAG that referenced it by number can be silently wrong.
- Multi-agent / multi-human race on the same queue, same vault, same numbering.
