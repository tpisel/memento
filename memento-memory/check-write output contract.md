---
title: check-write output contract
summary: "check-write emits the full PreToolUse harness verdict JSON on stdout itself (allow/deny/ask), not a bare {decision,reason_code,message} needing translation — so the PreToolUse hook is a true dumb pipe plus fail-closed-on-nonzero-exit. Resolves ADR-0031's ambiguous wording."
tags:
  - memento
  - enforcement
  - hooks
  - write
mode: living
status: reference
date: 2026-06-27
---

# check-write output contract

ADR-0031 describes `check-write`'s output two ways — "emits the harness verdict
JSON directly" and "returns `{decision, reason_code, message}`". These read as
conflicting. The implementation (memento-ryr.5) resolves it so the PreToolUse
wrapper can stay a genuine dumb pipe:

**`check-write` writes the full harness verdict JSON to stdout itself.** Shape:

```json
{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"<message>"},"reason_code":"read_only"}
```

- `decision` → `permissionDecision` (`allow` | `deny` | `ask`).
- `message` → `permissionDecisionReason`.
- `reason_code` is a **top-level extra field** (the harness ignores it) carried
  for the decision log (memento-ryr.19). The harness contract has no slot for it.

**Emission rules.** Only in-vault, file-targeted writes get a verdict:

- **allow** → `permissionDecision:"allow"` (auto-approves safe writes to the
  agent's own vault, matching ADR-0031's anti-friction intent).
- **deny / ask** → harness JSON with that decision; `ask` is used only for
  `vault_discovery_ambiguous`.
- **out-of-vault target, non-write tool, no vault** → **silent, exit 0**: never
  emit `allow` for these, or every Edit in the repo would auto-approve and bypass
  the user's normal permission flow.
- **internal failure** (unparseable payload, unsupported derivation for an
  in-vault target, IO/git error) → **non-zero exit**, no verdict. The dumb-pipe
  wrapper (memento-ryr.10) converts that to a fail-closed deny.

This is why the wrapper is `cat | memento check-write` plus a non-zero-exit →
deny rule, and nothing more: no JSON translation, no lattice, no message text in
bash. The latency gate (memento-ryr.18) must budget for the per-call `git
ls-files` ratification check inside `check-write`.
