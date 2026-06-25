#!/usr/bin/env bash
set -euo pipefail

# Claude Code SessionStart hook for the ADR-0025 A-UAT "orient hook" arm.
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
# script logic is needed. This file is intentionally inactive until a tester
# opts in through Claude Code settings.

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
repo_root="$(cd -- "${script_dir}/../.." && pwd -P)"

if command -v memento >/dev/null 2>&1; then
  orient_output="$(cd "$repo_root" && memento orient)"
else
  orient_output="$(cd "$repo_root" && go run ./cmd/memento orient)"
fi

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

printf '{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"%s"}}\n' "$(json_escape "$orient_output")"
