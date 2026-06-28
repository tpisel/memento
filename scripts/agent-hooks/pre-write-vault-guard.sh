#!/usr/bin/env bash
#
# PreToolUse hook (ADR-0031): a dumb pipe to `memento check-write`. All verdict
# logic — the mode lattice, the Bash command parse, every denial message — lives
# in unit-tested Go (internal/cli, internal/enforce). This wrapper forwards the
# raw PreToolUse payload on stdin to check-write and does exactly one thing of its
# own: it fails CLOSED. When check-write cannot return a verdict (binary missing,
# unparseable payload, IO/git error) it exits non-zero; this script turns that
# into a deny instead of letting the write through.
#
# It is deliberately NOT `set -euo pipefail`. Under `set -e` a non-zero
# check-write exit would propagate as the script's exit 1, which the harness
# treats as a *non-blocking* error and ALLOWS the write — the fail-OPEN bug this
# script exists to fix (ADR-0031, "Trust model and failure posture"). We read the
# exit code by hand instead.
#
# memento init (memento-ryr.12) installs the settings.json entry pointing at this
# script. Set MEMENTO_BIN to the memento binary if `memento` is not on PATH.

memento_bin="${MEMENTO_BIN:-memento}"

# Forward our stdin (the PreToolUse payload) straight to check-write. On a clean
# run check-write has already written the harness verdict JSON to our stdout, or
# stayed silent for an out-of-vault / non-write target; either way exit 0 is the
# verdict and we pass it through untouched.
"$memento_bin" check-write
status=$?
if [ "$status" -eq 0 ]; then
  exit 0
fi

# check-write could not produce a verdict. It writes stdout only on the verdict
# path, so nothing partial sits on our stdout here. Fail closed: emit a deny and
# exit 2. The harness blocks on exit 2 OR an explicit permissionDecision "deny";
# we send both so a JSON-only harness and an exit-code-only harness each block.
printf '%s\n' '{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"memento check-write could not run (missing binary, unparseable payload, or internal error), so this write is blocked fail-closed. Restore the memento hook before writing vault files."}}'
printf 'memento check-write unavailable (exit %s); blocking write fail-closed.\n' "$status" >&2
exit 2
