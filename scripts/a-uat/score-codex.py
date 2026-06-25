#!/usr/bin/env python3
"""Parse a `codex exec --json` transcript for one A-UAT cell and emit evidence
+ a provisional result, reusing the behavior rubric from score.py.

Codex is the A0-only row of the matrix: none of the Claude levers (orient hook,
write skill, vault guard) apply, so the only evidence is what the agent did on
its own. Codex's event model differs from claude's stream-json: shell commands
arrive as `command_execution` items and direct file edits as `file_change`
items (apply_patch) — the latter is codex's native-write surface, the B4/B5
leak equivalent of claude's Write/Edit. We normalise both into the same
evidence dict score.py already knows how to score.
"""
from __future__ import annotations

import json
import os
import re
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from score import bash_writes_into_vault, first_index, score  # noqa: E402

VAULT = "memento-memory"


def _cmd_string(item: dict) -> str:
    c = item.get("command", item.get("cmd", ""))
    if isinstance(c, list):
        return " ".join(str(x) for x in c)
    return str(c or "")


def load_codex(path: str):
    """Flatten codex events into ordered ('cmd', str) / ('file', path, kind)
    entries, plus terminal error metadata and the final agent message."""
    events = []
    final_text = ""
    err = {"is_error": False, "text": ""}
    with open(path, encoding="utf-8") as fh:
        for line in fh:
            line = line.strip()
            if not line:
                continue
            try:
                obj = json.loads(line)
            except json.JSONDecodeError:
                continue
            t = obj.get("type", "")
            item = obj.get("item", {}) if isinstance(obj.get("item"), dict) else {}
            itype = item.get("type", "")
            if t == "item.completed" and itype == "command_execution":
                events.append(("cmd", _cmd_string(item)))
            elif t in ("item.completed", "item.started") and itype == "file_change":
                # only count each change once (on completion)
                if t == "item.completed":
                    for ch in item.get("changes", []) or []:
                        events.append(("file", ch.get("path", ""), ch.get("kind", "")))
            elif t == "item.completed" and itype == "agent_message":
                final_text = item.get("text", "") or final_text
            # error surfaces: explicit error events, failed turns, or a credit/
            # rate message anywhere in the stream.
            blob = json.dumps(obj)
            turn_error = obj.get("error") if t.startswith("turn") else None
            if t in ("error", "turn.failed") or bool(turn_error):
                err["is_error"] = True
                err["text"] = (item.get("text") or obj.get("message") or str(turn_error) or blob)[:200]
            rate_source = " ".join(
                str(x)
                for x in (
                    err["text"],
                    item.get("text") if itype == "agent_message" else "",
                    obj.get("message") or "",
                    turn_error or "",
                )
            )
            if re.search(r"rate.?limit|quota|insufficient|usage limit|too many requests|429", rate_source, re.I):
                err["is_error"] = True
                err["text"] = err["text"] or rate_source[:200]
    return events, final_text, err


def analyze_codex(events):
    cmds = [e[1] for e in events if e[0] == "cmd"]
    files = [(e[1], e[2]) for e in events if e[0] == "file"]

    def any_cmd(pat):
        return any(re.search(pat, c) for c in cmds)

    file_vault = [p for p, _ in files if VAULT in p]
    ev = {
        "orient_called": any_cmd(r"memento\s+orient"),
        "orient_injected": False,  # no SessionStart hook on codex (A0)
        "brief_called": any_cmd(r"memento\s+brief"),
        "writing_read": any(re.search(r"memento\s+read", c) and "writing" in c for c in cmds),
        "memento_write": any_cmd(r"memento\s+write"),
        "native_vault_write": bool(file_vault) or any(bash_writes_into_vault(c) for c in cmds),
        "adr0026_native_edit": any("adr-0026" in p for p in file_vault)
        or any(("adr-0026" in c and bash_writes_into_vault(c)) for c in cmds),
        "guard_deny": False,  # no vault guard on codex (A0)
        "n_bash": len(cmds),
        "n_native": len(files),
    }
    orient_idx = first_index(events, lambda e: e[0] == "cmd" and re.search(r"memento\s+orient", e[1]))
    action_idx = first_index(
        events,
        lambda e: (e[0] == "cmd" and ("bd " in e[1] or re.search(r"memento\s+brief", e[1])))
        or e[0] == "file",
    )
    ev["orient_before_action"] = orient_idx is not None and (
        action_idx is None or orient_idx < action_idx
    )
    return ev


def main():
    if len(sys.argv) < 3:
        print("usage: score-codex.py <events.jsonl> <behavior>", file=sys.stderr)
        sys.exit(2)
    path, behavior = sys.argv[1], sys.argv[2]
    events, final_text, err = load_codex(path)
    ev = analyze_codex(events)

    is_rate = bool(re.search(r"rate.?limit|quota|insufficient|usage limit|429", err["text"], re.I))
    if err["is_error"] or not events:
        result, review = "error", True
        note = ("rate/credit limit: " if is_rate else "probe error/empty: ") + (
            err["text"][:120] or "no events captured"
        )
    else:
        result, review, note = score(behavior, False, ev)  # codex = A0, no vault guard

    print(json.dumps({
        "behavior": behavior,
        "result": result,
        "review": review,
        "note": note,
        "rate_limited": is_rate,
        "evidence": ev,
        "final_text_tail": final_text[-280:],
    }, indent=2))


if __name__ == "__main__":
    main()
