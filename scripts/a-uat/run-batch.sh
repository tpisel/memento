#!/usr/bin/env bash
set -euo pipefail

# A-UAT batch driver. Runs the core matrix (test-matrix.md "core set") cell by
# cell, resumably: it reads the run-report note and skips any cell already
# recorded for the current frozen_at, so it can be stopped (Ctrl-C, crash, or
# killed background job) and resumed by simply re-running it.
#
# Usage:   run-batch.sh
# Env:     MODELS="opus sonnet"   models to run (default both)
#          TRIALS="1 2 3"          trials per cell (default 3, matrix n=3)
#          DRY=1                   print the run/skip plan and exit
#          FROZEN=<commit>         override the matrix freeze commit
#
# Cell plan: each behavior is scored in the arms that move its primary lever,
# plus the all-on A7 (see matrix "Desired behaviors" / "Expectation pairs").

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
repo_root="$(git -C "$script_dir" rev-parse --show-toplevel)"
frozen=${FROZEN:-$(git -C "$repo_root" log -1 --format=%H -- memento-memory/a-uat/test-matrix.md)}
export FROZEN="$frozen"
short=${frozen:0:12}
report="$repo_root/memento-memory/a-uat/run-report.md"
models=${MODELS:-"opus sonnet"}
trials=${TRIALS:-"1 2 3"}

plan=(
  "A0:B1 B2 B3 B4 B5"   # baseline for every behavior
  "A1:B4 B5"            # vault guard on
  "A2:B1"               # orient hook on
  "A4:B3 B4"            # write skill on
  "A7:B1 B2 B3 B4 B5"   # all levers on
)

label() { case "$1" in opus) echo claude-opus ;; sonnet) echo claude-sonnet ;; *) echo "$1" ;; esac; }

done_cell() { # model_label arm behavior trial
  [ -f "$report" ] && grep -qF "\`$short\` | $1 | $2 | $3 | $4 |" "$report"
}

total=0 ran=0 skipped=0 failed=0
for model in $models; do
  ml="$(label "$model")"
  for entry in "${plan[@]}"; do
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
        if "$script_dir/run-cell.sh" "$model" "$arm" "$behavior" "$trial"; then
          ran=$((ran + 1))
        else
          failed=$((failed + 1))
          echo "!!! cell failed (continuing): $ml $arm $behavior t$trial"
        fi
      done
    done
  done
done
echo "=== batch: total=$total ran=$ran skipped=$skipped failed=$failed (frozen=$short) ==="
