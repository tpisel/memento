---
title: check-write decision log
summary: "The gitignored check-write audit is JSONL (one verdict per line) at .memento/decision-log.jsonl, recording exactly three enforcement-visible events — deny, drive_by_block, grant_consumption — and deliberately NOT plain allows. It is the enforcement audit, not a write journal; a log-write failure never flips a verdict."
tags:
  - memento
  - enforcement
  - hooks
  - observability
  - write
mode: living
status: reference
date: 2026-06-27
---

# check-write decision log

ADR-0031's observability bullet calls for a "structured, gitignored check-write
decision log (denials, grant consumptions, drive-by blocks)" — the audit the
retired write verb got for free via its commit trail. memento-ryr.19 implemented
it with these durable choices, none of which the ADR spells out:

- **Format: JSONL** at `.memento/decision-log.jsonl`, beside the unlock-grant and
  pending-write sidecars under the marker dir, gitignored the same way (manifest
  and config beside it stay tracked). One verdict per line means the writer only
  ever appends — no read-modify-rewrite that could race a concurrent verdict.
  `enforce.AppendDecisionLog` opens `O_APPEND|O_CREATE`.
- **Exactly three events**, the constants in `internal/enforce/decisionlog.go`:
  `deny` (ordinary content denial), `drive_by_block` (a deny broken out by its
  `drive_by_mode_change` reason so the audit distinguishes mode-tampering from a
  body denial), and `grant_consumption` (an allow that only stood because an
  active unlock waived the invariant on a *ratified* note).
- **Plain allows are NOT logged.** The log is the enforcement *audit*, not a write
  journal — recording every safe allow would bury the signal. A `grant_consumption`
  is logged only when `granted && ratified`: an unratified note allows regardless,
  so the grant was not actually consumed.
- **Best-effort.** A log-write failure must never flip a verdict (same rule as the
  pending-write ledger): it degrades to a missed line, surfaced on stderr only.
- **Single chokepoint + outer denials.** `computeVaultWriteVerdict` logs via defer,
  so every in-vault file verdict (Write/Edit/MultiEdit/Bash-append/apply_patch
  update) is covered uniformly; the denials emitted *outside* it — opaque Bash
  writes (no key) and apply_patch delete/rename — call `recordDecision` explicitly
  so the audit stays complete. The `vault_discovery_ambiguous` ask is not logged
  (no vault resolved, no key).

See also [[check-write output contract]]: `reason_code` rides on the stdout
verdict as a top-level extra field precisely so this log can record it.
