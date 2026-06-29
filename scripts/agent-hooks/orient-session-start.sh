#!/usr/bin/env bash
set -euo pipefail

# Claude Code SessionStart hook for the memento orient integration.
#
# Enable by pasting this block into .claude/settings.json and replacing the
# command path with this script's absolute path:
#
# {
#   "hooks": {
#     "SessionStart": [
#       {
#         "matcher": "startup|resume|compact",
#         "hooks": [
#           {
#             "type": "command",
#             "command": "/absolute/path/to/memento/scripts/agent-hooks/orient-session-start.sh"
#           }
#         ]
#       }
#     ]
#   }
# }
#
# The matcher source set makes the hook re-fire on resume/compact; no extra
# script logic is needed.

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
repo_root="$(cd -- "${script_dir}/../.." && pwd -P)"

json_escape() {
  local value=$1
  value=${value//\\/\\\\}
  value=${value//\"/\\\"}
  value=${value//$'\b'/\\b}
  value=${value//$'\f'/\\f}
  value=${value//$'\n'/\\n}
  value=${value//$'\r'/\\r}
  value=${value//$'\t'/\\t}
  printf '%s' "$value"
}

emit_context() {
  local context=$1
  printf '{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"%s"}}\n' "$(json_escape "$context")"
}

if command -v memento >/dev/null 2>&1; then
  mem=(memento)
else
  mem=(go run ./cmd/memento)
fi

compile_note=""
if ! compile_output="$(cd "$repo_root" && "${mem[@]}" compile 2>&1)"; then
  compile_note=$'memento compile failed; continuing with memento orient.\n'
fi
orient_output="$(cd "$repo_root" && "${mem[@]}" orient 2>&1)"

# Write-enforcement liveness (memento-mbd): emit it every SessionStart so
# 'enforcement: OFF' is unmissable, not something a human must run the verb to
# see. LIVE collapses to the headline; OFF keeps the full report so the break is
# actionable in place. A binary too old to know the verb cannot self-check, which
# is itself an enforcement-uncertainty signal. doctor's live-fire is in-process.
doctor_output="$(cd "$repo_root" && "${mem[@]}" doctor 2>&1)"
doctor_status=$?
case "$doctor_output" in
  *"vault write enforcement:"*)
    if [ "$doctor_status" -eq 0 ]; then
      doctor_note="${doctor_output%%$'\n'*}"$'\n'
    else
      doctor_note="$doctor_output"$'\n'
    fi
    ;;
  *)
    doctor_note=$'memento doctor unavailable; cannot confirm write enforcement is live (upgrade memento).\n'
    ;;
esac

emit_context "$doctor_note$compile_note$orient_output"
