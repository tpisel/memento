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

compile_note=""
if command -v memento >/dev/null 2>&1; then
  if ! compile_output="$(cd "$repo_root" && memento compile 2>&1)"; then
    compile_note=$'memento compile failed; continuing with memento orient.\n'
  fi
  orient_output="$(cd "$repo_root" && memento orient 2>&1)"
else
  if ! compile_output="$(cd "$repo_root" && go run ./cmd/memento compile 2>&1)"; then
    compile_note=$'memento compile failed; continuing with memento orient.\n'
  fi
  orient_output="$(cd "$repo_root" && go run ./cmd/memento orient 2>&1)"
fi

emit_context "$compile_note$orient_output"
