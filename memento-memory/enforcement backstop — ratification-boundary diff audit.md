---
title: Enforcement backstop — ratification-boundary diff audit
summary: "Hook-time mode enforcement (PreToolUse check-write) is necessarily partial: it only fires for write paths the agent surface exposes to hooks — so codex exec shell writes, untrusted codex hooks, and Claude opaque shell ($()/eval/interpreters) all fail open SILENTLY. The path-agnostic catch-all is a post-hoc audit of the on-disk diff vs ratified (git HEAD) state, reusing the pure EvaluatePrefixInvariant verdict, anchored at the git pre-commit hook (the ratification boundary). It is detection by default, optional commit-block as mitigation, and is independent of any agent's lifecycle-hook trust model."
tags:
  - memento
  - enforcement
  - hooks
  - codex
  - design
mode: living
status: reference
date: 2026-06-28
---

# Enforcement backstop — ratification-boundary diff audit

## The gap hook-time enforcement can't close

ADR-0031 enforces note `mode` at the **tool boundary**: a PreToolUse `check-write`
hook inspects the *intended* write and denies it. That only works for write paths the
agent surface routes through hooks, and the A-UAT runs (freeze `4634c7c`) proved the
boundary is leaky in three ways that all **fail open silently** — no deny, no
[[check-write compile drift handshake|drift alarm]], no log:

- **codex `exec` shell writes** — codex never fires PreToolUse for the shell tool (only
  `apply_patch`); a `printf > note.md` is unobserved. (Architectural; not matcher-fixable.)
- **untrusted codex hooks** — codex trusts hooks by content hash; until trusted the
  *entire* gate no-ops, not just shell. Bigger blast radius than the shell gap.
- **Claude opaque shell** — `check_write_bash.go` gates *recognisable* writes but lets
  variable-hidden paths / `$(...)` / `eval` / `python -c` fall through by design.

Observationally codex authors via `apply_patch` (0 raw shell writes across 144
transcripts), so practical exposure is low — but the *guarantee* shouldn't rest on a
behavioural propensity, and "modes are enforced" silently over-promises on these paths.

## The catch-all: audit the diff against ratified state

The diff is the right primitive because it is **path-agnostic** — it inspects
end-state, so it doesn't care whether bytes arrived via `apply_patch`, shell, opaque
redirection, codex, or a direct human edit. The existing pure verdict
`EvaluatePrefixInvariant(key, mode, old, new, …)` (`read-only ⟹ new==old`,
`append-only ⟹ new has old as prefix`, `living ⟹ allow`) already does the work; the
only change is the **source of `old`**: take it from **git HEAD (ratified state)**
rather than the pending-writes ledger (which only has gated writes).

For each *ratified* note (exists at HEAD) changed on disk, recompute the verdict with
`old = HEAD bytes`, `new = disk bytes`, honouring the same authorisation context
check-write composes (active unlock grant, `write-mode` mode change). Any violation
with no covering grant is an **ungated mode violation** → alarm.

## Anchor it at the ratification boundary (pre-commit)

`compile` already runs in PostToolUse *and* the git pre-commit hook. The **pre-commit
hook is the true catch-all**:

- it sees the full working-tree-vs-HEAD diff — every change, every surface, every tool;
- it is a **git hook, independent of codex's lifecycle-hook trust model** — so it still
  fires when codex hooks are untrusted (whole-gate-off) or a codex version bump silently
  disables the gate;
- it sits exactly at **ratification** — in memento's model a change isn't durable until
  committed, so auditing there stops an unauthorised change from becoming *permanent*
  silently, even if it briefly existed on disk.

One mechanism backstops all three silent fail-opens. Detection by default; since nothing
is ratified pre-commit, it *can* be mitigation (exit non-zero → block the commit) without
violating the model. See [[adr-0031-remove-write-verb-hook-enforced-native-writes]].

## Honest limits

- **Commit-time, not real-time** — catches before ratification, not before bytes touch
  disk. For a git-backed vault where commit = durability, that is the right boundary, but
  it is not a real-time wall.
- **False-positive engineering is the work** — must skip brand-new notes (birth, not at
  HEAD), unlock-grant-covered changes (read the sidecar *before* prepare-commit-msg clears
  it), legitimate `write-mode` mode changes, and compile's own frontmatter/manifest
  normalisation.
- **`git commit --no-verify` skips it** — fine for honest-agent + signal, not tamper-proof.
- **Only fires on commit** — uncommitted unauthorised edits linger until then (the
  PostToolUse tier catches earlier where it fires).
- **Mitigation rides on the host hook's `set -e`** — when another tool owns
  `core.hooksPath` (e.g. beads' `.beads/hooks`), init composes memento's step into the
  *effective* hook git runs rather than the dead `.git/hooks/pre-commit` (memento-42o).
  memento's own default hook ships `#!/bin/sh` + `set -eu`, so a non-zero `memento
  compile` aborts the commit; a foreign host hook without `set -e` (beads' generated
  hook has none) runs the appended block and *swallows* the non-zero exit. So in the
  composed case DETECTION still alarms but the optional commit-block MITIGATION
  (`MEMENTO_STRICT_COMMIT`) silently no-ops. doctor's `precommit-anchor-live` confirms
  reachability (detection live), not exit-propagation (mitigation live) — to make
  mitigation robust the composed block would need to self-propagate compile's exit
  (`memento compile || exit $?`) rather than rely on the host's shell options.

## Posture shift

With this backstop, "codex enforcement is `apply_patch`-scoped" stops being a silent gap
and becomes: **`apply_patch` gated in real-time, everything else caught at commit.**
Implementation tracked in the aligned bead under epic `memento-ryr`.
