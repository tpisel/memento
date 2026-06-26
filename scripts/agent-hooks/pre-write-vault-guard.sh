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
#         "matcher": "Write|Edit|MultiEdit|Bash",
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
  # Emit an absolute, symlink-resolved path with forward-slash separators. On
  # Windows (Git Bash) realpath yields backslashes, but the vault prefix/glob
  # comparisons below are written with '/', so normalize here at the single
  # entry point for resolved paths. No-op on POSIX where os.sep is already '/'.
  local path=$1
  python3 -c 'import os, sys; print(os.path.realpath(os.path.abspath(sys.argv[1])).replace(os.sep, "/"))' "$path"
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

bash_writes_vault() {
  local vault_root=$1
  local command=$2

  python3 - "$vault_root" "$command" <<'PY'
import os
import shlex
import sys

vault_root = os.path.realpath(os.path.abspath(sys.argv[1]))
command = sys.argv[2]
cwd = os.getcwd()

try:
    lexer = shlex.shlex(command, posix=True, punctuation_chars=True)
    lexer.whitespace_split = True
    lexer.commenters = ""
    tokens = list(lexer)
except ValueError:
    tokens = command.replace(">>", " >> ").replace(">", " > ").split()

boundaries = {";", "&&", "||", "|", "&", "(", ")"}


def is_boundary(token):
    return token in boundaries


def segment_after(index):
    out = []
    for token in tokens[index + 1:]:
        if is_boundary(token):
            break
        out.append(token)
    return out


def under_vault(value):
    if not value:
        return False
    path = os.path.expanduser(value)
    if not os.path.isabs(path):
        path = os.path.join(cwd, path)
    resolved = os.path.realpath(os.path.abspath(path))
    return resolved == vault_root or resolved.startswith(vault_root + os.sep)


def optionless(args):
    result = []
    after_options = False
    for arg in args:
        if arg == "--":
            after_options = True
            continue
        if not after_options and arg.startswith("-") and arg != "-":
            continue
        result.append(arg)
    return result


def has_sed_inplace(args):
    return any(arg == "-i" or arg.startswith("-i") for arg in args)


def has_perl_inplace(args):
    flag_text = "".join(arg[1:] for arg in args if arg.startswith("-") and arg != "--")
    return "p" in flag_text and "i" in flag_text


for index, token in enumerate(tokens):
    if token in {">", ">>"}:
        if index + 1 < len(tokens) and under_vault(tokens[index + 1]):
            sys.exit(0)
        continue

    command_name = os.path.basename(token)
    args = segment_after(index)

    if command_name == "tee":
        for arg in optionless(args):
            if under_vault(arg):
                sys.exit(0)
    elif command_name in {"cp", "mv"}:
        operands = optionless(args)
        if len(operands) >= 2 and under_vault(operands[-1]):
            sys.exit(0)
    elif command_name == "sed":
        if has_sed_inplace(args) and any(under_vault(arg) for arg in optionless(args)):
            sys.exit(0)
    elif command_name == "perl":
        if has_perl_inplace(args) and any(under_vault(arg) for arg in optionless(args)):
            sys.exit(0)

sys.exit(1)
PY
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
command="$(printf '%s' "$payload" | extract_payload_field command)"
deny_reason="Memento vault files must be changed with memento write so note modes are enforced. If memento write rejects a protected note, ask the user before using --force-with-reason."

case "$tool_name" in
  ""|Write|Edit|MultiEdit|Bash)
    ;;
  *)
    exit 0
    ;;
esac

if [ "$tool_name" = "Bash" ] && [ -z "$command" ]; then
  exit 0
fi
if [ "$tool_name" != "Bash" ] && [ -z "$file_path" ]; then
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

if [ "$tool_name" = "Bash" ]; then
  if bash_writes_vault "$vault_root" "$command"; then
    permission_decision "deny" "$deny_reason"
  fi
  exit 0
fi

target_path="$(resolve_path "$file_path")"

case "$target_path" in
  "$vault_root"|"$vault_root"/*)
    permission_decision "deny" "$deny_reason"
    ;;
  *)
    exit 0
    ;;
esac
