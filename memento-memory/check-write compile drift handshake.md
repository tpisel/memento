---
title: check-write↔compile drift handshake
summary: "The PreToolUse gate and PostToolUse compile coordinate through a gitignored .memento/pending-writes.json ledger: check-write records the expected post-write bytes-hash per key on an allow; compile hashes what landed, raises a loud DRIFT ALARM on mismatch, then clears the ledger. Bash appends record nothing (landed bytes not derivable). compile stays exit-0 on drift; the PostToolUse hook turns the alarm token into exit 2 to surface it."
tags:
  - memento
  - enforcement
  - hooks
  - compile
  - write
mode: living
status: reference
date: 2026-06-27
---

# check-write↔compile drift handshake

ADR-0031 makes PostToolUse `compile` "coherence **+ drift alarm**": the detective
backstop under the predictive PreToolUse gate, and (with the decision log) the
only integrity signal until a doctor verb exists. memento-ryr.11 implements the
two-process handshake concretely.

**The ledger.** `.memento/pending-writes.json` — a gitignored sidecar beside the
unlock-grant one (`key → "sha256:…"`, full-file bytes-hash). `enforce.HashBytes`
hashes **whole-file** content (what a Write lands / an Edit replay produces), not
the post-frontmatter body the manifest's `body_sha` covers — the two are
deliberately distinct.

**Record (check-write, PreToolUse).** On an `allow` verdict for a tool whose
new-bytes it derived exactly — Write/Edit/MultiEdit — check-write records
`key → HashBytes(newBytes)`. A record failure never flips allow→deny; the gate's
verdict stands and the handshake degrades to a missed drift check.

**Bash records nothing.** A `>>` append is allowed but its landed bytes are not
statically derivable (the gate models a synthetic suffix only). Bash PostToolUse
also carries no path. So a Bash write leaves no expectation and PostToolUse just
recompiles (idempotent). This is the realization of ADR-0031's open question
"PostToolUse path-gating for Bash is not achievable".

**Verify (compile, PostToolUse).** compile hashes the bytes now on disk for each
ledger key and compares. Mismatch — or the file never landing — prints a loud
`DRIFT ALARM: <key> …` on stderr. After the pass it clears the **whole** ledger,
so each expectation is checked once and the alarm fires once. A stale expectation
from an approved-but-not-performed write therefore self-clears on the next
compile (one false alarm, not a permanent one).

**Loudness split (deliberate).** `compile` stays **exit 0** on drift (banner only)
so it never fails an unrelated manual `compile` or the pre-commit hook. The
PostToolUse wrapper (`scripts/agent-hooks/post-write-compile.sh`) greps compile's
stderr for the `DRIFT ALARM` token and, only then, exits **2** — the PostToolUse
code that feeds stderr back to the agent. Detector and surfacer are split across
the two processes.

Related: [[check-write output contract]] (the PreToolUse verdict shape);
[[adr-0031-remove-write-verb-hook-enforced-native-writes]] (§"New-bytes
derivation — and the replay caveat"); [[adr-0022-auto-compile-after-write]]
(the coherence intent re-homed onto PostToolUse).
