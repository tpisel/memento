#!/usr/bin/env bash
set -euo pipefail

# Post-ADR-0031 A-UAT cell runner. Runs one model x arm x behavior x trial cell
# headless against a frozen commit, captures the tool-use transcript plus two
# out-of-band evidence sources (the b19 check-write decision log and a post-run
# vault git diff), scores it, and appends one row to the append-only run-report.
#
# The matrix has two whole-build arms (see test-matrix.md), compared per ADR-0031's
# validation gate ("write-verb build vs. hooks-only build side by side"):
#   W = the last commit where `memento write` existed (pre-removal), no enforcement
#       hooks installed — the leak-rate control / prior world.
#   H = the branch tip: native writes under the real SessionStart orient hook +
#       PreToolUse check-write gate + PostToolUse compile drift alarm.
# Both arms run on Claude AND codex (ADR-0031 brought codex into enforcement scope).
#
# Usage: run-cell.sh <model: opus|sonnet|codex> <arm: W|H> <behavior: N1..N6> <trial>
# Env:   TIMEOUT=<secs>    per-probe wall clock (default: 420)
#        REPORT_KEY=<key>  vault-relative report note (default: a-uat/run-report.md)
#        W_FROZEN=<commit> override the W (pre-removal) commit
#        H_FROZEN=<commit> override the H build commit (default: matrix freeze)
#        CODEX_MODEL=<m>   codex model alias (default: gpt-5.5)

model=${1:?model}; arm=${2:?arm}; behavior=${3:?behavior}; trial=${4:?trial}
timeout_secs=${TIMEOUT:-420}
report_key=${REPORT_KEY:-a-uat/run-report.md}

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
repo_root="$(git -C "$script_dir" rev-parse --show-toplevel)"

# W is pinned to the last commit where the write verb existed (memento-ryr.13, the
# parent of the ryr.14 removal). H tracks the matrix freeze (the matrix's
# last-touched commit) so the cell key matches the run's pre-registration anchor.
W_FROZEN_DEFAULT=690b23cb15cdb6258d23b7672b4f76538dc8c700
case "$arm" in
  W) frozen=${W_FROZEN:-$W_FROZEN_DEFAULT}; enforce=0; orient=0 ;;
  H) frozen=${H_FROZEN:-$(git -C "$repo_root" log -1 --format=%H -- memento-memory/a-uat/test-matrix.md)}; enforce=1; orient=1 ;;
  *) echo "unknown arm: $arm (expected W or H)" >&2; exit 2 ;;
esac

case "$model" in
  opus) model_alias=opus; model_label=claude-opus ;;
  sonnet) model_alias=sonnet; model_label=claude-sonnet ;;
  codex) model_alias=${CODEX_MODEL:-gpt-5.5}; model_label=codex ;;
  *) echo "unknown model: $model" >&2; exit 2 ;;
esac

# N6 has no prompt of its own: the matrix runs it AS the N1 probe (same happy
# native-write task) and differs only at scoring time, where the codex scorer checks
# SessionStart orient-injection instead of the write landing. Route N6 → N1.txt so
# the codex-H N6 cells actually run rather than aborting on a missing prompt.
prompt_behavior="$behavior"
[ "$behavior" = N6 ] && prompt_behavior=N1
prompt_file="$script_dir/prompts/$prompt_behavior.txt"
[ -f "$prompt_file" ] || { echo "no prompt for $behavior" >&2; exit 2; }

# Build (and cache) the arm's memento binary FROM ITS OWN COMMIT: W's binary has the
# write verb, H's has check-write/compile/write-mode/unlock. The hooks invoke this
# same binary, so enforcement is exercised exactly as the arm shipped it.
bin_dir="$script_dir/.bin/${frozen:0:12}"
bin="$bin_dir/memento"
if [ ! -x "$bin" ]; then
  echo ">>> building memento @ ${frozen:0:12} (arm $arm)" >&2
  build_wt="$(mktemp -d)"
  git -C "$repo_root" worktree add --detach "$build_wt" "$frozen" >/dev/null
  mkdir -p "$bin_dir"
  ( cd "$build_wt" && go build -o "$bin" ./cmd/memento )
  git -C "$repo_root" worktree remove --force "$build_wt" >/dev/null 2>&1 || true
  rm -rf "$build_wt"
fi

work="$(mktemp -d)"
wt="$work/wt"
cleanup() {
  git -C "$repo_root" worktree remove --force "$wt" >/dev/null 2>&1 || true
  rm -rf "$work"
}
trap cleanup EXIT

git -C "$repo_root" worktree add --detach "$wt" "$frozen" >/dev/null

# Preserve probe blindness: the A-UAT apparatus (matrix + run report) must not be
# visible in the worktree, or a probe exploring a-uat/ could read its own test plan.
# Removal is uniform across cells and affects no probe task (probes only ever touch
# the target notes named in their prompt). The scorer's vault-diff parser drops
# these two keys so their deletion is never miscounted as a probe write.
rm -f "$wt/memento-memory/a-uat/test-matrix.md" "$wt/memento-memory/a-uat/run-report.md"

# Put the arm binary on PATH as `memento` (the orient hook resolves it by name) and
# export MEMENTO_BIN for the guard/compile dumb-pipe scripts.
export PATH="$bin_dir:$PATH"
export MEMENTO_BIN="$bin"

stream="$work/stream.jsonl"
stderr_log="$work/stderr.log"
status=ok

if [ "$model" = codex ]; then
  # codex reads config + hooks from $CODEX_HOME. Wire the real hooks for H; W gets
  # an empty config (no enforcement). codex trusts hooks by content hash and skips
  # untrusted ones, so an enforced run must bypass trust for this vetted automation
  # (see the matrix "codex hook-trust" caveat) — without it the H-codex gate
  # silently no-ops and the cell degrades to a W-like ungated run.
  export CODEX_HOME="$wt/.codex"
  mkdir -p "$CODEX_HOME"
  if [ "$enforce" = 1 ]; then
    cat > "$CODEX_HOME/config.toml" <<EOF
# A-UAT codex config (ADR-0031). hooks key must be top-level.
hooks = "hooks.json"
EOF
    cat > "$CODEX_HOME/hooks.json" <<EOF
{
  "SessionStart": [{"matcher":"startup|resume|compact","hooks":[{"type":"command","command":"$wt/scripts/agent-hooks/orient-session-start.sh","timeout_sec":30}]}],
  "PreToolUse": [{"matcher":"apply_patch|Shell","hooks":[{"type":"command","command":"$wt/scripts/agent-hooks/pre-write-vault-guard.sh","timeout_sec":5}]}],
  "PostToolUse": [{"matcher":"apply_patch|Shell","hooks":[{"type":"command","command":"$wt/scripts/agent-hooks/post-write-compile.sh","timeout_sec":30}]}]
}
EOF
    trust_flag=(--dangerously-bypass-hook-trust)
  else
    printf '# A-UAT codex config (W arm: no enforcement hooks)\n' > "$CODEX_HOME/config.toml"
    trust_flag=()
  fi
  ( cd "$wt" && timeout "$timeout_secs" codex exec - \
      --json \
      --ephemeral \
      --disable memories \
      --sandbox workspace-write \
      --model "$model_alias" \
      ${trust_flag[@]+"${trust_flag[@]}"} \
      -c 'approval_policy="never"' \
      -c 'web_search="disabled"' \
      < "$prompt_file" \
      > "$stream" 2> "$stderr_log" ) || status="exit=$?"
else
  # Claude arm. Scoped allowlist (default permission mode) so the probe acts
  # autonomously while PreToolUse hooks still fire. Hooks point at the worktree's
  # own copies so they operate on the worktree, not the main repo.
  hooks_json=""
  if [ "$enforce" = 1 ]; then
    hooks_json="\"PreToolUse\":[{\"matcher\":\"Write|Edit|MultiEdit|Bash\",\"hooks\":[{\"type\":\"command\",\"command\":\"$wt/scripts/agent-hooks/pre-write-vault-guard.sh\"}]}],\"PostToolUse\":[{\"matcher\":\"Write|Edit|MultiEdit|Bash\",\"hooks\":[{\"type\":\"command\",\"command\":\"$wt/scripts/agent-hooks/post-write-compile.sh\"}]}]"
  fi
  if [ "$orient" = 1 ]; then
    [ -n "$hooks_json" ] && hooks_json+=","
    hooks_json+="\"SessionStart\":[{\"matcher\":\"startup|resume|compact\",\"hooks\":[{\"type\":\"command\",\"command\":\"$wt/scripts/agent-hooks/orient-session-start.sh\"}]}]"
  fi
  settings="$work/settings.json"
  cat > "$settings" <<EOF
{
  "permissions": {"allow": ["Bash", "Read", "Write", "Edit", "MultiEdit"]},
  "hooks": {$hooks_json}
}
EOF
  ( cd "$wt" && timeout "$timeout_secs" claude -p "$(cat "$prompt_file")" \
      --model "$model_alias" \
      --output-format stream-json --include-hook-events --verbose \
      --settings "$settings" \
      > "$stream" 2> "$stderr_log" ) || status="exit=$?"
fi

# Persist the transcript + both evidence sources out of the worktree first, so a
# scoring bug can never lose the evidence (the worktree and $work are removed on
# exit). The decision log is gitignored and lives under the vault marker dir; the
# vault diff captures what actually landed on disk.
keep_dir="$repo_root/scripts/a-uat/runs"
mkdir -p "$keep_dir"
stamp="$(date +%Y%m%dT%H%M%S)"
base="$keep_dir/${stamp}_${model_label}_${arm}_${behavior}_t${trial}"
keep="${base}.jsonl"
decision_log="${base}.decisionlog.jsonl"
vault_diff="${base}.vaultdiff.txt"
cp "$stream" "$keep"
cp "$wt/memento-memory/.memento/decision-log.jsonl" "$decision_log" 2>/dev/null || : > "$decision_log"
git -C "$wt" status --porcelain -- memento-memory > "$vault_diff" 2>/dev/null || : > "$vault_diff"

if [ "$model" = codex ]; then
  scored="$(python3 "$script_dir/score-codex.py" "$stream" "$behavior" "$arm" "$decision_log" "$vault_diff")"
else
  scored="$(python3 "$script_dir/score.py" "$stream" "$behavior" "$arm" "$decision_log" "$vault_diff")"
fi
rate_limited=$(printf '%s' "$scored" | python3 -c 'import json,sys; print("1" if json.load(sys.stdin)["rate_limited"] else "0")')
# A session/rate limit means nothing useful ran. Do not append a polluting row
# (the cell stays "not done" and is retried); signal the batch to stop with 3.
if [ "$rate_limited" = 1 ]; then
  echo "cell: $model_label $arm $behavior t$trial -> RATE-LIMITED; stopping (re-run to resume). log: $keep" >&2
  exit 3
fi
result=$(printf '%s' "$scored" | python3 -c 'import json,sys; print(json.load(sys.stdin)["result"])')
review=$(printf '%s' "$scored" | python3 -c 'import json,sys; print("yes" if json.load(sys.stdin)["review"] else "no")')
note_txt=$(printf '%s' "$scored" | python3 -c 'import json,sys; print(json.load(sys.stdin)["note"])')
evidence=$(printf '%s' "$scored" | python3 -c '
import json,sys
o=json.load(sys.stdin); e=o["evidence"]; la=e.get("leak",{})
flags=[k for k in ("orient_called","orient_injected","convention_read","write_verb","write_mode_verb","unlock_verb","native_vault_write","drift_alarm") if e.get(k)]
leak=[k for k in ("hard_bypass","silent_leak","false_deny") if la.get(k)]
parts=("; ".join(flags) if flags else "no key tool-use")
if leak: parts += " | LEAK: "+",".join(leak)
parts += " (bash={},native={},retry={},changed={})".format(e.get("n_bash"),e.get("n_native"),e.get("retry_after_deny"),len(e.get("changed",{})))
print(parts)
')

sanitize() { printf '%s' "$1" | tr '|\n' '/ '; }
row="| \`${frozen:0:12}\` | $model_label | $arm | $behavior | $trial | $result | $review | $(sanitize "$evidence") — $(sanitize "$note_txt") [$status] | log: \`${keep#"$repo_root"/}\` |"

# Append the row natively (ADR-0031: no write verb). The run-report is append-only;
# this harness append does not need a recompile (manifest coherence is the
# PostToolUse hook's job in agent runs, not here).
report_path="$repo_root/memento-memory/$report_key"
mkdir -p "$(dirname "$report_path")"
printf '%s\n' "$row" >> "$report_path"

echo "cell: $model_label $arm $behavior t$trial -> $result (review=$review) [$status]"
echo "  evidence: $evidence"
echo "  log: $keep"
