#!/usr/bin/env bash
set -euo pipefail

# Post-ADR-0031 A-UAT batch driver. Runs the H-only matrix (test-matrix.md) cell by
# cell, resumably: it reads the run-report note and skips any cell already recorded
# (as a successful [ok] row) for that arm's frozen commit, so it can be stopped
# (Ctrl-C, crash, killed background job) and resumed by re-running it.
#
# Usage:   run-batch.sh
# Env:     MODELS="opus sonnet codex"   models to run (default opus sonnet codex)
#          TRIALS="1 2 3"               trials per cell (default 3, matrix n=3)
#          DRY=1                        print the run/skip plan and exit
#          H_FROZEN                     override the H arm's commit (see run-cell.sh)
#          W_FROZEN                     W arm commit for an ad-hoc baseline (off the plan)
#
# Cell plan (ryr.29): only the shipping arm H runs by default — the confounded W/H
# A/B was dropped (rule-1 is now an absolute leak bar, not a W non-regression). The
# models also diverge on the tunnel/recovery axis but not on raw leak, so opus runs
# the full behaviour set and sonnet is a single N4 spot check:
#   opus   N1-N5            (n=3, the friction-surfacing model)
#   sonnet N4 only          (n=1 spot check)
#   codex  N1-N5 + N6       (n=3; N6 = codex SessionStart orient-injection check)
# => 12 cells (5 + 1 + 6). W is no longer on the plan but run-cell.sh still builds
# it on explicit request (`run-cell.sh opus W N2 1`) for an ad-hoc baseline.

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
repo_root="$(git -C "$script_dir" rev-parse --show-toplevel)"
report="$repo_root/memento-memory/a-uat/run-report.md"
models=${MODELS:-"opus sonnet codex"}
trials=${TRIALS:-"1 2 3"}

W_FROZEN_DEFAULT=690b23cb15cdb6258d23b7672b4f76538dc8c700
w_frozen=${W_FROZEN:-$W_FROZEN_DEFAULT}
h_frozen=${H_FROZEN:-$(git -C "$repo_root" log -1 --format=%H -- memento-memory/a-uat/test-matrix.md)}
export W_FROZEN="$w_frozen" H_FROZEN="$h_frozen"

opus_plan=(
  "H:N1 N2 N3 N4 N5"
)
sonnet_plan=(
  "H:N4"                  # spot check: models diverge on the tunnel/recovery axis, not raw leak
)
codex_plan=(
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

total=0 ran=0 skipped=0 failed=0 stopped=0 cells=0
for model in $models; do
  ml="$(label "$model")"
  # Per-model plan + trial count: opus full N1-N5 (n from TRIALS), sonnet a single
  # N4 spot check (n=1 — first TRIALS token so a narrow override still resolves),
  # codex full + N6. W is not planned here; pass it to run-cell.sh directly instead.
  case "$model" in
    sonnet) active_plan=("${sonnet_plan[@]}"); model_trials="${trials%% *}" ;;
    codex) active_plan=("${codex_plan[@]}"); model_trials="$trials" ;;
    *) active_plan=("${opus_plan[@]}"); model_trials="$trials" ;;
  esac
  for entry in "${active_plan[@]}"; do
    arm="${entry%%:*}"; behaviors="${entry#*:}"
    for behavior in $behaviors; do
      cells=$((cells + 1))
      for trial in $model_trials; do
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
if [ "${DRY:-0}" = 1 ]; then
  echo "=== DRY plan: $cells cells / $total sessions (H=${h_frozen:0:12}) ==="
elif [ "$stopped" = 1 ]; then
  echo "=== STOPPED on rate/session limit: ran=$ran skipped=$skipped failed=$failed. Re-run to resume. ==="
else
  echo "=== batch complete: $cells cells, total=$total ran=$ran skipped=$skipped failed=$failed (H=${h_frozen:0:12}) ==="
fi
