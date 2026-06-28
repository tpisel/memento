---
title: Loosening-justification persistence — decision log, not commit trailers
summary: "Where the *why* of a mode loosening lives. Decided 2026-06-28: write-mode loosenings append (with justification) to the existing gitignored .memento/ check-write decision log; NO commit trailers; the Memento-Unlock: trailer is RETIRED; unlock justifications are intentionally NOT persisted (we don't care to). Rationale: a committed/trailer audit would aspire to a binding-audit property the rest of the system explicitly disclaims (cooperative guardrail, not security control). The integrity floor stays the committed mode-line diff + the pre-commit MODE VIOLATION audit, not the justification text. Consumption deferred to a future audit faculty."
tags:
  - memento
  - enforcement
  - unlock
  - write-mode
  - audit
  - design
mode: living
status: reference
date: 2026-06-28
---

# Loosening-justification persistence — decision log, not commit trailers

## The question

Loosening a note's mode (`write-mode … living`, `unlock`) takes a `--justification`.
Where should that *why* durably live, and who consumes it? ADR-0031 originally put
the `unlock` justification into a `Memento-Unlock:` git commit trailer
(see [[unlock-grant trailer lift runs in prepare-commit-msg]]); `write-mode`'s
justification went nowhere (stderr only) — an inverted asymmetry where the more
consequential, *permanent* escalation had the weaker trail.

## Decision (2026-06-28)

- **`write-mode` loosenings append to the existing gitignored
  `.memento/` check-write decision log**, carrying the justification. The log gains
  a justification field and a loosening event kind; `write-mode` (which does not
  touch the log today) is wired into it. See [[check-write decision log]].
- **No commit trailers.** The `Memento-Unlock:` trailer is **retired** — and with
  it the `prepare-commit-msg` hook. Grant-clearing (the re-lock) moves to the
  `pre-commit` hook, where `compile` already runs.
- **`unlock` justifications are intentionally NOT persisted.** The grant sidecar
  holds the reason until the grant is cleared at commit, then it is gone. We
  consciously do not carry it forward.

## Why gitignored-local is the *consistent* choice, not a compromise

Mode enforcement is declared a **cooperative-agent guardrail, not a security
control**; the integrity floor is the committed diff + drift / MODE-VIOLATION
detection, *not* metadata. A committed audit trail (file or trailer) would be the
one over-engineered corner aspiring to a binding-audit property the rest of the
system disclaims. A gitignored local log matches the posture: it helps an honest
agent / operator reconstruct what happened, it does not pretend to bind a dishonest
one. It also fits the existing gitignored `.memento/` sidecars (grants,
pending-writes, decision log) rather than inventing a new category, and a tool that
lives **inside someone else's repo** should keep its footprint in its own subtree,
not in the shared commit-message namespace that changelog / Conventional-Commits /
release tooling all parse.

## The cost we accepted, named

Gitignored = local-only: the *why* is **not in PR review and does not survive a
fresh clone**. This is fine because the loosening *act* is already visible in review
without it:

- **`write-mode`** — the mode flip is a committed frontmatter diff
  (`mode: read-only` → `mode: living`); the integrity-relevant signal is durable and
  in-review. The justification is genuinely supplementary.
- **`unlock`** — the only place this is weaker. An unlock leaves no committed marker
  of authorisation, so PR review cannot mechanically distinguish an authorised thaw
  from a silent gate-bypass. We took **answer 1**: we don't carry a committed signal,
  because the norm is "*any* edit to a ratified read-only note in a diff gets
  questioned in review regardless," and the pre-commit MODE VIOLATION audit already
  fires loudly at commit time on the acting machine.

## Consumption — deferred

Nothing in memento reads the log back today; it is a local observability surface. A
programmatic "show me this note's loosening history" belongs to the future **`audit`**
faculty (open-world epistemic integrity), **not** `doctor` (mechanical health) — see
[[review-audit-doctor faculties]] and [[doctor-scoping]].

## Related

- [[adr-0031-remove-write-verb-hook-enforced-native-writes]] — the ADR this amends
  (its "unlock grant and its audit trail" section + addendum reverse here).
- [[unlock-grant trailer lift runs in prepare-commit-msg]] — the trailer mechanism
  this retires.
- [[check-write decision log]] — the gitignored log loosenings now append to.
