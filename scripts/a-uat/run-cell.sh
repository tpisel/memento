#!/usr/bin/env bash
set -euo pipefail

# A-UAT cell runner (Claude arms). Runs one model x arm x behavior x trial cell
# headless against a frozen commit, captures the tool-use transcript, scores it
# with score.py, and appends one row to the append-only run-report vault note.
#
# This is the intervention half of the runner (observation lives in score.py).
# It reproduces an ADR-0025 lever arm faithfully by injecting that arm's hooks
# and skill into an isolated git worktree via `claude --settings`, rather than
# mutating the repo. The probe sees only the frozen prompt Z — never the matrix,
# the expectations, or the scoring — so it cannot game the test.
#
# Usage: run-cell.sh <model: opus|sonnet> <arm: A0..A7> <behavior: B1..B5> <trial>
# Env:   FROZEN=<commit>   commit to base the worktree on (default: HEAD)
#        TIMEOUT=<secs>    per-probe wall clock (default: 420)
#        REPORT_KEY=<key>  vault-relative report note (default: a-uat/run-report.md)

model=${1:?model}; arm=${2:?arm}; behavior=${3:?behavior}; trial=${4:?trial}
timeout_secs=${TIMEOUT:-420}
report_key=${REPORT_KEY:-a-uat/run-report.md}

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
repo_root="$(git -C "$script_dir" rev-parse --show-toplevel)"
# frozen_at tracks the matrix's freeze (its last-touched commit), not HEAD, so
# unrelated harness commits don't shift the key that resumability is built on.
frozen=${FROZEN:-$(git -C "$repo_root" log -1 --format=%H -- memento-memory/a-uat/test-matrix.md)}

case "$model" in
  opus) model_alias=opus; model_label=claude-opus ;;
  sonnet) model_alias=sonnet; model_label=claude-sonnet ;;
  *) echo "unknown model: $model" >&2; exit 2 ;;
esac

# Arm -> lever flags (matrix "Variation arms" table).
case "$arm" in
  A0) skill=0; orient=0; guard=0 ;;
  A1) skill=0; orient=0; guard=1 ;;
  A2) skill=0; orient=1; guard=0 ;;
  A3) skill=0; orient=1; guard=1 ;;
  A4) skill=1; orient=0; guard=0 ;;
  A5) skill=1; orient=0; guard=1 ;;
  A6) skill=1; orient=1; guard=0 ;;
  A7) skill=1; orient=1; guard=1 ;;
  *) echo "unknown arm: $arm" >&2; exit 2 ;;
esac

prompt_file="$script_dir/prompts/$behavior.txt"
[ -f "$prompt_file" ] || { echo "no prompt for $behavior" >&2; exit 2; }

work="$(mktemp -d)"
wt="$work/wt"
cleanup() {
  git -C "$repo_root" worktree remove --force "$wt" >/dev/null 2>&1 || true
  rm -rf "$work"
}
trap cleanup EXIT

git -C "$repo_root" worktree add --detach "$wt" "$frozen" >/dev/null

# Preserve probe blindness: the A-UAT apparatus (the matrix and the run report)
# must not be visible in the worktree, or a probe that explores a-uat/ could
# read its own test plan and game it. Removing them is uniform across all cells
# and does not affect any probe task (probes only ever create new a-uat notes).
rm -f "$wt/memento-memory/a-uat/test-matrix.md" "$wt/memento-memory/a-uat/run-report.md"

# Build the arm's settings. Scoped allowlist (default permission mode) so the
# probe acts autonomously while PreToolUse hooks still fire. Hooks point at the
# worktree's own copies so they operate on the worktree, not the main repo.
hooks_json=""
sep=""
if [ "$orient" = 1 ]; then
  hooks_json+="$sep\"SessionStart\":[{\"matcher\":\"startup|resume|compact\",\"hooks\":[{\"type\":\"command\",\"command\":\"$wt/scripts/agent-hooks/orient-session-start.sh\"}]}]"
  sep=","
fi
if [ "$guard" = 1 ]; then
  hooks_json+="$sep\"PreToolUse\":[{\"matcher\":\"Write|Edit|MultiEdit\",\"hooks\":[{\"type\":\"command\",\"command\":\"$wt/scripts/agent-hooks/pre-write-vault-guard.sh\"}]}]"
fi

settings="$work/settings.json"
cat > "$settings" <<EOF
{
  "permissions": {"allow": ["Bash", "Read", "Write", "Edit", "MultiEdit"]},
  "hooks": {$hooks_json}
}
EOF

if [ "$skill" = 1 ]; then
  mkdir -p "$wt/.claude/skills/memento-write"
  cp "$wt/memento-memory/_memento/skills/write.md" "$wt/.claude/skills/memento-write/SKILL.md"
fi

stream="$work/stream.jsonl"
stderr_log="$work/stderr.log"
status=ok
( cd "$wt" && timeout "$timeout_secs" claude -p "$(cat "$prompt_file")" \
    --model "$model_alias" \
    --output-format stream-json --include-hook-events --verbose \
    --settings "$settings" \
    > "$stream" 2> "$stderr_log" ) || status="exit=$?"

# Persist the transcript out of the worktree first, so a scoring bug can never
# lose the evidence (the worktree and $work are removed on exit).
keep_dir="$repo_root/scripts/a-uat/runs"
mkdir -p "$keep_dir"
stamp="$(date +%Y%m%dT%H%M%S)"
keep="$keep_dir/${stamp}_${model_label}_${arm}_${behavior}_t${trial}.jsonl"
cp "$stream" "$keep"

scored="$(python3 "$script_dir/score.py" "$stream" "$behavior" "$guard")"
result=$(printf '%s' "$scored" | python3 -c 'import json,sys; print(json.load(sys.stdin)["result"])')
review=$(printf '%s' "$scored" | python3 -c 'import json,sys; print("yes" if json.load(sys.stdin)["review"] else "no")')
note_txt=$(printf '%s' "$scored" | python3 -c 'import json,sys; print(json.load(sys.stdin)["note"])')
evidence=$(printf '%s' "$scored" | python3 -c '
import json,sys
e=json.load(sys.stdin)["evidence"]
flags=[k for k in ("orient_called","orient_injected","brief_called","writing_read","memento_write","native_vault_write","adr0026_native_edit","guard_deny") if e.get(k)]
print(("; ".join(flags) if flags else "no key tool-use") + " (bash={},native={})".format(e["n_bash"], e["n_native"]))
')

sanitize() { printf '%s' "$1" | tr '|\n' '/ '; }
row="| \`${frozen:0:12}\` | $model_label | $arm | $behavior | $trial | $result | $review | $(sanitize "$evidence") — $(sanitize "$note_txt") [$status] | log: \`${keep#"$repo_root"/}\` |"

printf '%s\n' "$row" | ( cd "$repo_root" && go run ./cmd/memento write "$report_key" >/dev/null )

echo "cell: $model_label $arm $behavior t$trial -> $result (review=$review) [$status]"
echo "  evidence: $evidence"
echo "  log: $keep"
