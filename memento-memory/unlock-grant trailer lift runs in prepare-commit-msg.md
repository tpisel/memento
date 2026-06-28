---
title: unlock-grant trailer lift runs in prepare-commit-msg
summary: "ADR-0031 says 'the pre-commit hook lifts pending grant justifications into a Memento-Unlock: commit trailer before clearing them' — but a git pre-commit hook runs before the commit message exists and cannot write a trailer. The lift+clear is implemented as a prepare-commit-msg hook (memento lift-grants \"$1\"): the only stage that runs after pre-commit succeeds AND owns the message file. pre-commit still does compile + git add manifest; prepare-commit-msg owns the trailer + grant clear."
tags:
  - memento
  - enforcement
  - hooks
  - unlock
  - git
mode: living
status: reference
date: 2026-06-27
---

# unlock-grant trailer lift runs in prepare-commit-msg

> **Superseded (2026-06-28).** The `Memento-Unlock:` commit trailer is being
> **retired** — see [[loosening justification persistence]] and the ADR-0031
> 2026-06-28 addendum. Loosening justifications now go to the gitignored
> `.memento/` decision log (`write-mode`) or are intentionally not persisted
> (`unlock`); the `prepare-commit-msg` hook is removed and grant-clearing (the
> re-lock) moves to `pre-commit`. The note below records the now-historical
> mechanism and *why* `prepare-commit-msg` was needed *while the trailer existed*;
> the re-lock semantics it documents still hold, only the hook that performs them
> changes.

ADR-0031 §"The `unlock` grant and its audit trail" states the **pre-commit hook**
lifts pending grant justifications into a `Memento-Unlock:` commit trailer before
clearing them. That is a factual impossibility in git: a `pre-commit` hook fires
**before the commit message exists**, gets no message-file argument, and cannot
write a trailer. Taken literally the ADR's mechanism does not work.

**Correction (memento-ryr.13).** The lift+clear is implemented as a
**`prepare-commit-msg`** hook, not pre-commit. That stage is the only one that
both (a) runs *after* `pre-commit` has succeeded — so a failed compile still
aborts before any grant is touched — and (b) receives the commit message file as
`$1`, which a trailer must be written into. The block is
`memento lift-grants "$1"` under the same `command -v memento` guard as the
pre-commit block.

The two git hooks split cleanly:

- **pre-commit** — `memento compile` + `git add` the manifest (unchanged; that
  work genuinely belongs pre-commit, where staging is possible).
- **prepare-commit-msg** — `memento lift-grants "$1"`: load grants, append one
  `Memento-Unlock: <key>: <justification>` trailer per grant via
  `git interpret-trailers --in-place`, then `ClearGrants` (drops the whole
  sidecar). No grants ⇒ message untouched (the steady-state fast path). A corrupt
  sidecar ⇒ exit 1 (fail-closed; `set -eu` aborts the commit rather than silently
  clearing the audit trail).

**Why prepare-commit-msg and not commit-msg or post-commit.** `post-commit` is too
late — the commit object is already sealed, so a trailer added there never lands.
`commit-msg` would also work mechanically, but `prepare-commit-msg` is the
conventional "fill in the message" stage and, usefully, is **not** skipped by
`git commit --no-verify` (which bypasses pre-commit and commit-msg) — so "any
commit clears all grants" holds even for `--no-verify`.

**Decided semantics preserved.** Any commit clears ALL pending grants; committing
re-ratifies, and for an already-ratified read-only note the **grant deletion**
(not ratification) is what re-locks it. The trailer is the only durable record of
the *why*, since the sidecar is gitignored and commit-cleared.

Related: [[adr-0031-remove-write-verb-hook-enforced-native-writes]] (the ADR this
corrects); [[check-write compile drift handshake]] (the sibling gitignored
`.memento/` sidecar and its pre-commit-adjacent lifecycle).
