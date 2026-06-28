#!/usr/bin/env python3
"""Score one post-ADR-0031 A-UAT cell from a `codex exec --json` transcript,
reusing score.py's behaviour rubric and leak cross-reference.

ADR-0031 brought codex into scope for *enforcement*, not just adherence: codex-cli
ships a lifecycle-hooks engine whose deny contract is byte-identical to Claude's,
so codex runs the H (hooks-only) arm under real enforcement (the W write-verb build
stays available ad-hoc but is off the default plan). Codex
edits via `apply_patch` (file_change items) and shell (`command_execution` items);
file_change is its native-write surface, the leak equivalent of Claude's
Write/Edit. We normalise both into the evidence dict score.py knows how to score,
then layer in the same decision-log + vault-diff cross-reference.

Caveat carried from the matrix: codex trusts hooks by content hash and skips
untrusted ones, so an H-codex run only exercises enforcement if the runner trusted
the staged hooks first; otherwise the gate silently no-ops and the cell degrades to
a W-like ungated run. The orient SessionStart check (N6) exists precisely because
the codex contract proves byte-identity only for the PreToolUse deny verdict, not
that SessionStart additionalContext actually injects.
"""
from __future__ import annotations

import json
import os
import re
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from score import (  # noqa: E402
    build_evidence_from_codex,
    read_optional,
    score,
)

VAULT = "memento-memory"


def _cmd_string(item: dict) -> str:
    c = item.get("command", item.get("cmd", ""))
    if isinstance(c, list):
        return " ".join(str(x) for x in c)
    return str(c or "")


def load_codex(path: str):
    """Flatten codex events into ordered ('cmd', str) / ('file', path, kind)
    entries, plus terminal error metadata, the final agent message, and the raw
    stream text (for orient-injection and drift-alarm detection)."""
    events = []
    final_text = ""
    raw = ""
    err = {"is_error": False, "text": ""}
    with open(path, encoding="utf-8") as fh:
        for line in fh:
            raw += line
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
            elif t == "item.completed" and itype == "file_change":
                for ch in item.get("changes", []) or []:
                    events.append(("file", ch.get("path", ""), ch.get("kind", "")))
            elif t == "item.completed" and itype == "agent_message":
                txt = item.get("text", "")
                # Keep agent prose in stream order for the loosening stop-and-confirm check.
                events.append(("text", txt))
                final_text = txt or final_text
            blob = json.dumps(obj)
            turn_error = obj.get("error") if t.startswith("turn") else None
            # Rate/credit limits live ONLY on the error / turn.failed paths. Agent
            # prose (agent_message text) is the probe's narration — a denial-recovery
            # probe (N2-N5) naturally says "insufficient"/"quota"/"rate", and scoring
            # that as a rate limit would set rc=3 and abort the whole batch. So scope
            # error + rate detection to the error envelope, never the model's output.
            if t in ("error", "turn.failed") or bool(turn_error):
                err["is_error"] = True
                err["text"] = (item.get("text") or obj.get("message") or str(turn_error) or blob)[:200]
    return events, final_text, err, raw


def main():
    if len(sys.argv) < 4:
        print(
            "usage: score-codex.py <events.jsonl> <behavior:N1..N6> <arm:W|H> "
            "[decision-log.jsonl] [vault-diff.txt]",
            file=sys.stderr,
        )
        sys.exit(2)
    path, behavior, arm = sys.argv[1], sys.argv[2], sys.argv[3]
    decision_log = sys.argv[4] if len(sys.argv) > 4 else ""
    vault_diff = sys.argv[5] if len(sys.argv) > 5 else ""

    events, final_text, err, raw = load_codex(path)
    ev = build_evidence_from_codex(
        events, raw, behavior, read_optional(decision_log), read_optional(vault_diff)
    )

    is_rate = bool(re.search(r"rate.?limit|quota|insufficient|usage limit|429", err["text"], re.I))
    if err["is_error"] or not events:
        result, review = "error", True
        note = ("rate/credit limit: " if is_rate else "probe error/empty: ") + (
            err["text"][:120] or "no events captured"
        )
    else:
        result, review, note = score(behavior, arm, ev)

    print(json.dumps({
        "behavior": behavior,
        "arm": arm,
        "result": result,
        "review": review,
        "note": note,
        "rate_limited": is_rate,
        "evidence": ev,
        "final_text_tail": final_text[-280:],
    }, indent=2, default=str))


if __name__ == "__main__":
    main()
