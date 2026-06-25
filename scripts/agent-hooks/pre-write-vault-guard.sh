#!/usr/bin/env bash
set -euo pipefail

# Claude Code PreToolUse hook for the ADR-0025 A-UAT "write-side enforcement"
# arm. This artifact is intentionally inactive until a tester opts in through
# Claude Code settings; memento init must not install or reference it yet.
#
# Enable by pasting this block into .claude/settings.json and replacing the
# command path with this script's absolute path:
#
# {
#   "hooks": {
#     "PreToolUse": [
#       {
#         "matcher": "Write|Edit|MultiEdit",
#         "hooks": [
#           {
#             "type": "command",
#             "command": "/absolute/path/to/memento/scripts/agent-hooks/pre-write-vault-guard.sh"
#           }
#         ]
#       }
#     ]
#   }
# }
#
# Optional: set MEMENTO_VAULT_ROOT, or pass the vault root as argv[1], to avoid
# marker discovery. Without either, the script discovers the single .memento
# marker under MEMENTO_REPO_ROOT, git's top-level directory, or this repo root.

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
default_repo_root="$(cd -- "${script_dir}/../.." && pwd -P)"

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

permission_decision() {
  local decision=$1
  local reason=$2
  printf '{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"%s","permissionDecisionReason":"%s"}}\n' "$decision" "$(json_escape "$reason")"
}

resolve_path() {
  local path=$1
  python3 -c 'import os, sys; print(os.path.realpath(os.path.abspath(sys.argv[1])))' "$path"
}

extract_payload_field() {
  local field=$1
  python3 -c '
import json
import sys

field = sys.argv[1]
try:
    payload = json.load(sys.stdin)
except json.JSONDecodeError:
    sys.exit(0)

if field == "tool_name":
    value = payload.get("tool_name", "")
else:
    value = payload.get("tool_input", {}).get(field, "")

if isinstance(value, str):
    print(value)
' "$field"
}

discover_vault_root() {
  local repo_root=${MEMENTO_REPO_ROOT:-}

  if [ -z "$repo_root" ] && command -v git >/dev/null 2>&1; then
    repo_root="$(git rev-parse --show-toplevel 2>/dev/null || true)"
  fi
  if [ -z "$repo_root" ]; then
    repo_root=$default_repo_root
  fi
  repo_root="$(resolve_path "$repo_root")"

  local markers=()
  while IFS= read -r marker; do
    markers+=("$marker")
  done < <(find "$repo_root" -path '*/.git' -prune -o -type d -name .memento -print)

  case ${#markers[@]} in
    0)
      return 1
      ;;
    1)
      dirname -- "${markers[0]}"
      ;;
    *)
      return 2
      ;;
  esac
}

payload="$(cat)"
tool_name="$(printf '%s' "$payload" | extract_payload_field tool_name)"
file_path="$(printf '%s' "$payload" | extract_payload_field file_path)"

case "$tool_name" in
  ""|Write|Edit|MultiEdit)
    ;;
  *)
    exit 0
    ;;
esac

if [ -z "$file_path" ]; then
  exit 0
fi

vault_root=${MEMENTO_VAULT_ROOT:-${1:-}}
if [ -z "$vault_root" ]; then
  if vault_root="$(discover_vault_root)"; then
    :
  else
    status=$?
    if [ "$status" -eq 2 ]; then
      permission_decision "ask" "Memento vault discovery found multiple .memento markers. Set MEMENTO_VAULT_ROOT, or use memento write so mode checks apply."
    fi
    exit 0
  fi
fi

vault_root="$(resolve_path "$vault_root")"
target_path="$(resolve_path "$file_path")"

case "$target_path" in
  "$vault_root"|"$vault_root"/*)
    permission_decision "deny" "Memento vault files must be changed with memento write so the note mode check applies."
    ;;
  *)
    exit 0
    ;;
esac
