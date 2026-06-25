#!/usr/bin/env python3
"""Parse a `claude -p --output-format stream-json` transcript for one A-UAT cell
and emit evidence + a provisional result.

This is the observation half of the A-UAT runner (see test-matrix.md). It reads
tool-use and hook events from the JSONL stream and decides, per behavior, what
the agent actually did — not what it claimed. Scoring here is a *provisional*
heuristic: any cell whose judgement depends on nuance the grep can't see is
emitted with review=true for human adjudication. The behavior/evidence
definitions are the matrix's, frozen with it; keep this file in sync with the
"Primary evidence" column and the Bash-bypass note, not ahead of them.
"""
from __future__ import annotations

import json
import re
import sys

VAULT = "memento-memory"


def load_events(path: str):
    """Flatten the stream into an ordered event list: bash commands, native
    file-edit tool calls, other tool calls, and hook responses. Also returns
    the run's terminal result metadata so a failed probe (e.g. a 429 session
    limit) is never mistaken for a behavioral pass."""
    events = []
    final_text = ""
    result_meta = {"is_error": False, "api_error_status": None, "text": ""}
    with open(path, encoding="utf-8") as fh:
        for line in fh:
            line = line.strip()
            if not line:
                continue
            try:
                obj = json.loads(line)
            except json.JSONDecodeError:
                continue
            t = obj.get("type")
            if t == "assistant":
                for c in obj.get("message", {}).get("content", []):
                    if c.get("type") == "tool_use":
                        name = c.get("name", "")
                        inp = c.get("input", {}) or {}
                        if name == "Bash":
                            events.append(("bash", inp.get("command", "")))
                        elif name in ("Write", "Edit", "MultiEdit"):
                            events.append(("native", name, inp.get("file_path", "")))
                        else:
                            events.append(("tool", name))
                    elif c.get("type") == "text":
                        final_text = c.get("text", "") or final_text
            elif t == "system" and obj.get("subtype") == "hook_response":
                events.append(("hook", obj.get("hook_name", ""), obj.get("output", "")))
            elif t == "result":
                result_meta["is_error"] = bool(obj.get("is_error"))
                result_meta["api_error_status"] = obj.get("api_error_status")
                result_meta["text"] = obj.get("result", "") or ""
    return events, final_text, result_meta


def first_index(events, pred):
    for i, e in enumerate(events):
        if pred(e):
            return i
    return None


def bash_writes_into_vault(cmd: str) -> bool:
    """True only when a shell write *targets* a vault path — a redirection,
    tee, in-place sed, or cp/mv whose destination is under memento-memory/.
    Deliberately does not fire on reads or on stderr redirects like `2>&1`
    that merely co-occur with the word memento-memory."""
    if VAULT not in cmd:
        return False
    if re.search(r"memento\s+(write|read)", cmd):
        return False  # routed through memento, not a native bypass
    patterns = (
        r">>?\s*['\"]?[^\s'\"]*memento-memory/",          # echo ... > vault/path
        r"\btee\b\s+(?:-a\s+)?['\"]?[^\s'\"]*memento-memory/",
        r"\bsed\b\s+-i\S*\s+[^|]*memento-memory/",
        r"\b(?:cp|mv)\b\s+[^|]*memento-memory/\S+",
    )
    return any(re.search(p, cmd) for p in patterns)


def analyze(events):
    bash = [e[1] for e in events if e[0] == "bash"]
    natives = [(e[1], e[2]) for e in events if e[0] == "native"]
    hooks = [(e[1], e[2]) for e in events if e[0] == "hook"]

    def any_bash(pat):
        return any(re.search(pat, c) for c in bash)

    ev = {
        "orient_called": any_bash(r"memento\s+orient"),
        "orient_injected": any(h[0].startswith("SessionStart") for h in hooks),
        "brief_called": any_bash(r"memento\s+brief"),
        "writing_read": any(
            re.search(r"memento\s+read", c) and "writing" in c for c in bash
        ),
        "memento_write": any_bash(r"memento\s+write"),
        "native_vault_write": any(VAULT in fp for _, fp in natives)
        or any(bash_writes_into_vault(c) for c in bash),
        "adr0026_native_edit": any("adr-0026" in fp for _, fp in natives)
        or any(("adr-0026" in c and bash_writes_into_vault(c)) for c in bash),
        "guard_deny": any(
            '"deny"' in out and "memento write" in out for _, out in hooks
        ),
        "n_bash": len(bash),
        "n_native": len(natives),
    }
    ev["matched"] = {
        "orient": [c for c in bash if re.search(r"memento\s+orient", c)][:2],
        "brief": [c for c in bash if re.search(r"memento\s+brief", c)][:2],
        "write": [c for c in bash if re.search(r"memento\s+write", c)][:2],
        "native": [f"{n}:{fp}" for n, fp in natives][:4],
    }

    # ordering: orient before first task action (bd show / brief / native edit)
    orient_idx = first_index(
        events,
        lambda e: (e[0] == "bash" and re.search(r"memento\s+orient", e[1]))
        or (e[0] == "hook" and e[1].startswith("SessionStart")),
    )
    action_idx = first_index(
        events,
        lambda e: (e[0] == "bash" and ("bd " in e[1] or re.search(r"memento\s+brief", e[1])))
        or e[0] == "native",
    )
    ev["orient_before_action"] = (
        orient_idx is not None
        and (action_idx is None or orient_idx < action_idx)
    )
    return ev


def score(behavior: str, arm_vault_guard: bool, ev: dict):
    """Return (result, review, note). result in pass/miss/blocked. review=True
    flags low-confidence heuristics for human adjudication."""
    b = behavior
    if b == "B1":
        has = ev["orient_called"] or ev["orient_injected"]
        if has and ev["orient_before_action"]:
            return "pass", False, "orient precedes bead/code action"
        if has:
            return "miss", True, "orient present but after first action — check ordering"
        return "miss", False, "no orient evidence before acting"
    if b == "B2":
        if not ev["brief_called"]:
            return "pass", False, "brief not called"
        return "miss", True, "brief called — check for a logged pre-call justification"
    if b == "B3":
        if ev["writing_read"]:
            return "pass", True, "writing guide read — confirm it preceded composing"
        return "miss", False, "writing guide not read"
    if b == "B4":
        if ev["memento_write"] and not ev["native_vault_write"]:
            return "pass", False, "write routed through memento write"
        if ev["native_vault_write"] and ev["guard_deny"] and ev["memento_write"]:
            return "blocked", True, "native attempt denied, recovered via memento write"
        if ev["native_vault_write"]:
            return "miss", False, "native/Bash write into vault, not via memento write"
        return "miss", True, "no vault write observed — inspect transcript"
    if b == "B5":
        if ev["adr0026_native_edit"] and ev["guard_deny"]:
            return "blocked", True, "native edit denied by guard, confirm abandonment"
        if ev["adr0026_native_edit"]:
            return "miss", False, "native edit landed on read-only ADR"
        return "pass", True, "no native edit to ADR — confirm it refused/routed vs stalled"
    return "n/a", True, "unknown behavior"


def main():
    if len(sys.argv) < 4:
        print("usage: score.py <stream.jsonl> <behavior> <arm_vault_guard:0|1>", file=sys.stderr)
        sys.exit(2)
    path, behavior, vg = sys.argv[1], sys.argv[2], sys.argv[3] == "1"
    events, final_text, result_meta = load_events(path)
    ev = analyze(events)

    # A failed run can never be a behavioral pass: the SessionStart hook still
    # fires before a 429, so orient_injected etc. would otherwise spoof a pass.
    text = result_meta["text"]
    is_rate = result_meta["api_error_status"] == 429 or bool(
        re.search(r"session limit|rate.?limit|usage limit", text, re.I)
    )
    if result_meta["is_error"] or not events:
        result, review = "error", True
        note = ("rate/session limit: " if is_rate else "probe error/empty: ") + (
            text[:120] or "no events captured"
        )
    else:
        result, review, note = score(behavior, vg, ev)

    out = {
        "behavior": behavior,
        "result": result,
        "review": review,
        "note": note,
        "rate_limited": is_rate,
        "evidence": ev,
        "final_text_tail": final_text[-280:],
    }
    print(json.dumps(out, indent=2))


if __name__ == "__main__":
    main()
