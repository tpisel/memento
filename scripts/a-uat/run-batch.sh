#!/usr/bin/env bash
set -euo pipefail

# Post-ADR-0031 A-UAT batch driver. Runs the W-vs-H matrix (test-matrix.md) cell by
# cell, resumably: it reads the run-report note and skips any cell already recorded
# (as a successful [ok] row) for that arm's frozen commit, so it can be stopped
# (Ctrl-C, crash, killed background job) and resumed by re-running it.
#
# Usage:   run-batch.sh
# Env:     MODELS="opus sonnet codex"   models to run (default opus sonnet codex)
#          TRIALS="1 2 3"               trials per cell (default 3, matrix n=3)
#          DRY=1                        print the run/skip plan and exit
#          W_FROZEN / H_FROZEN          override either arm's commit (see run-cell.sh)
#
# Cell plan: both whole-build arms (W control, H enforced) run every behaviour on
# every model; the codex SessionStart injection check N6 runs only on codex H.

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
repo_root="$(git -C "$script_dir" rev-parse --show-toplevel)"
report="$repo_root/memento-memory/a-uat/run-report.md"
models=${MODELS:-"opus sonnet codex"}
trials=${TRIALS:-"1 2 3"}

W_FROZEN_DEFAULT=690b23cb15cdb6258d23b7672b4f76538dc8c700
w_frozen=${W_FROZEN:-$W_FROZEN_DEFAULT}
h_frozen=${H_FROZEN:-$(git -C "$repo_root" log -1 --format=%H -- memento-memory/a-uat/test-matrix.md)}
export W_FROZEN="$w_frozen" H_FROZEN="$h_frozen"

claude_plan=(
  "W:N1 N2 N3 N4 N5"
  "H:N1 N2 N3 N4 N5"
)
codex_plan=(
  "W:N1 N2 N3 N4 N5"
  "H:N1 N2 N3 N4 N5 N6"   # N6 = codex SessionStart orient-injection check
)

label() { case "$1" in opus) echo claude-opus ;; sonnet) echo claude-sonnet ;; *) echo "$1" ;; esac; }
arm_short() { case "$1" in W) echo "${w_frozen:0:12}" ;; H) echo "${h_frozen:0:12}" ;; esac; }

# A cell counts as done only if it has a *successful* ([ok]) row for that arm's
# frozen commit; error rows (e.g. a 429 that slipped through) do not count.
done_cell() { # model_label arm behavior trial
  [ -f "$report" ] || return 1
  grep -F "\`$(arm_short "$2")\` | $1 | $2 | $3 | $4 |" "$report" | grep -q '\[ok\]'
}

total=0 ran=0 skipped=0 failed=0 stopped=0
for model in $models; do
  ml="$(label "$model")"
  case "$model" in
    codex) active_plan=("${codex_plan[@]}") ;;
    *) active_plan=("${claude_plan[@]}") ;;
  esac
  for entry in "${active_plan[@]}"; do
    arm="${entry%%:*}"; behaviors="${entry#*:}"
    for behavior in $behaviors; do
      for trial in $trials; do
        total=$((total + 1))
        if done_cell "$ml" "$arm" "$behavior" "$trial"; then
          skipped=$((skipped + 1))
          [ "${DRY:-0}" = 1 ] && echo "skip  $ml $arm $behavior t$trial"
          continue
        fi
        if [ "${DRY:-0}" = 1 ]; then
          echo "RUN   $ml $arm $behavior t$trial"
          continue
        fi
        echo ">>> ($((ran + 1)) run / $skipped skipped) $ml $arm $behavior t$trial"
        rc=0
        "$script_dir/run-cell.sh" "$model" "$arm" "$behavior" "$trial" || rc=$?
        if [ "$rc" -eq 0 ]; then
          ran=$((ran + 1))
        elif [ "$rc" -eq 3 ]; then
          stopped=1; break 4   # rate/session limit: stop; re-run later to resume
        else
          failed=$((failed + 1))
          echo "!!! cell failed (continuing): $ml $arm $behavior t$trial"
        fi
      done
    done
  done
done
if [ "$stopped" = 1 ]; then
  echo "=== STOPPED on rate/session limit: ran=$ran skipped=$skipped failed=$failed. Re-run to resume. ==="
else
  echo "=== batch complete: total=$total ran=$ran skipped=$skipped failed=$failed (W=${w_frozen:0:12} H=${h_frozen:0:12}) ==="
fi
