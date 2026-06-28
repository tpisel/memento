#!/usr/bin/env bash
#
# PostToolUse hook (ADR-0031, re-homing ADR-0022's auto-compile off the deleted
# `write` verb). After a vault write lands it runs `memento compile`, which does
# two jobs: keep the manifest/brief coherent with disk, and run the compile half
# of the check-write handshake — compare what landed against the bytes-hash the
# PreToolUse gate recorded, raise a DRIFT ALARM on mismatch, then clear the
# ledger.
#
# Unlike the PreToolUse guard this hook CANNOT block: by PostToolUse the write
# has already happened. It is best-effort coherence plus detection, so a compile
# failure is not fatal here. It does not parse the payload to gate on the target:
# Bash PostToolUse carries no path, so we always recompile (idempotent); the
# matcher `memento init` installs scopes which tools fire this at all.
#
# The one signal worth surfacing is drift. `memento compile` prints the alarm on
# stderr and exits 0 (so it never fails an unrelated `compile` or the pre-commit
# hook). This wrapper watches for the alarm token and, only then, exits 2 — the
# PostToolUse code that feeds stderr back to the agent — so a detected tamper or
# replay divergence is loud where it happened, not buried in a transcript.
#
# Set MEMENTO_BIN to the memento binary if `memento` is not on PATH.

memento_bin="${MEMENTO_BIN:-memento}"

# Capture compile's stderr so we can scan it for the alarm, then re-emit it
# verbatim. compile writes nothing to stdout, so discarding it loses nothing.
compile_err="$("$memento_bin" compile 2>&1 1>/dev/null)"
if [ -n "$compile_err" ]; then
  printf '%s\n' "$compile_err" >&2
fi

if printf '%s' "$compile_err" | grep -q 'DRIFT ALARM'; then
  exit 2
fi
exit 0
