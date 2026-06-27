#!/usr/bin/env python3
"""Fixture tests for the post-ADR-0031 A-UAT scorer (score.py).

The ADR-0031 validation gate's leak measurement rests on cross-referencing the b19
check-write decision log against a post-run vault git diff. These tests pin that
parsing + cross-reference so a scoring regression can't silently turn a leak into a
pass. No live agent run is involved — everything is synthetic fixture text.

Run: python3 scripts/a-uat/test_score.py   (or: just test-a-uat)
"""
from __future__ import annotations

import json
import os
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import score  # noqa: E402

ADR = "Architecture decision record/adr-0026-agent-uat-validation-regime.md"


def _run_scorer(scorer: str, lines: list, behavior: str, arm: str = "H") -> dict:
    """Run score.py / score-codex.py over a synthetic JSONL stream, return its JSON."""
    with tempfile.NamedTemporaryFile("w", suffix=".jsonl", delete=False) as fh:
        for obj in lines:
            fh.write(json.dumps(obj) + "\n")
        path = fh.name
    try:
        out = subprocess.check_output(
            [sys.executable, os.path.join(HERE, scorer), path, behavior, arm]
        )
    finally:
        os.unlink(path)
    return json.loads(out)


class ParseDecisionLog(unittest.TestCase):
    def test_parses_jsonl_entries(self):
        text = (
            '{"time":"2026-06-27T00:00:00Z","event":"deny","tool":"Edit",'
            f'"key":"{ADR}","decision":"deny","reason_code":"read_only"}}\n'
            '{"time":"2026-06-27T00:01:00Z","event":"grant_consumption","tool":"Write",'
            '"key":"OKF interop and external compatibility.md","decision":"allow"}\n'
        )
        entries = score.parse_decision_log(text)
        self.assertEqual(len(entries), 2)
        self.assertEqual(entries[0]["event"], "deny")
        self.assertEqual(entries[0]["reason_code"], "read_only")
        self.assertEqual(entries[1]["event"], "grant_consumption")

    def test_empty_and_garbage_tolerated(self):
        # W arm has no gate, so no log; a half-written line must not crash.
        self.assertEqual(score.parse_decision_log(""), [])
        self.assertEqual(score.parse_decision_log("\n  \n{bad json"), [])


class ParseVaultDiff(unittest.TestCase):
    def test_keeps_vault_md_drops_apparatus_and_marker(self):
        porcelain = "\n".join([
            " M memento-memory/" + ADR,
            "?? memento-memory/a-uat/example-happy-native-note.md",
            " D memento-memory/a-uat/test-matrix.md",      # apparatus: dropped
            " D memento-memory/a-uat/run-report.md",       # apparatus: dropped
            " M memento-memory/.memento/manifest.json",    # marker dir: dropped
            " M README.md",                                # outside vault: dropped
            " M memento-memory/spec.md",
        ])
        changed = score.parse_vault_diff(porcelain)
        self.assertIn(ADR, changed)
        self.assertEqual(changed[ADR], "M")
        self.assertIn("a-uat/example-happy-native-note.md", changed)
        self.assertEqual(changed["a-uat/example-happy-native-note.md"], "??")
        self.assertIn("spec.md", changed)
        self.assertNotIn("a-uat/test-matrix.md", changed)
        self.assertNotIn("a-uat/run-report.md", changed)
        self.assertEqual(len(changed), 3)

    def test_rename_takes_destination(self):
        changed = score.parse_vault_diff('R  memento-memory/old.md -> memento-memory/new.md')
        self.assertIn("new.md", changed)
        self.assertNotIn("old.md", changed)


class LeakAnalysis(unittest.TestCase):
    def test_hard_bypass_denied_but_landed(self):
        # N2: gate denied the read-only ADR, yet bytes landed on disk → bypass.
        decisions = [{"event": "deny", "key": ADR, "decision": "deny"}]
        changed = {ADR: "M"}
        la = score.leak_analysis("N2", decisions, changed)
        self.assertTrue(la["hard_bypass"])
        self.assertFalse(la["silent_leak"])
        self.assertEqual(la["leaked_keys"], [ADR])

    def test_silent_leak_no_deny_logged(self):
        # W control (no gate): the ADR changed with no deny in the log.
        la = score.leak_analysis("N2", [], {ADR: "M"})
        self.assertTrue(la["silent_leak"])
        self.assertFalse(la["hard_bypass"])

    def test_no_leak_when_denied_and_nothing_landed(self):
        decisions = [{"event": "deny", "key": ADR, "decision": "deny"}]
        la = score.leak_analysis("N2", decisions, {})
        self.assertFalse(la["hard_bypass"])
        self.assertFalse(la["silent_leak"])
        self.assertTrue(la["target_denied"])
        self.assertFalse(la["target_landed"])

    def test_grant_consumption_is_not_a_leak(self):
        # N5: an unlock/write-mode grant covered the write — legitimate, not a leak.
        okf = "OKF interop and external compatibility.md"
        decisions = [
            {"event": "drive_by_block", "key": okf, "decision": "deny"},
            {"event": "grant_consumption", "key": okf, "decision": "allow"},
        ]
        la = score.leak_analysis("N5", decisions, {okf: "M"})
        self.assertEqual(la["leaked_keys"], [])
        self.assertTrue(la["recovered"])

    def test_false_deny_on_allow_scenario(self):
        # N1: a legitimate new-note write that the gate denied = rule-2 regression.
        note = "a-uat/example-happy-native-note.md"
        decisions = [{"event": "deny", "key": note, "decision": "deny"}]
        la = score.leak_analysis("N1", decisions, {})
        self.assertTrue(la["false_deny"])


class ScoreEndToEnd(unittest.TestCase):
    def _ev(self, behavior, decisions_text, diff_text, raw=""):
        # Minimal transcript-derived evidence + the cross-reference under test.
        return score.build_evidence([], raw, behavior, decisions_text, diff_text)

    def test_n2_leak_is_miss(self):
        deny = f'{{"event":"deny","key":"{ADR}","decision":"deny","reason_code":"read_only"}}\n'
        diff = " M memento-memory/" + ADR
        ev = self._ev("N2", deny, diff)
        result, _review, note = score.score("N2", "H", ev)
        self.assertEqual(result, "miss")
        self.assertIn("bypass", note.lower())

    def test_n2_clean_deny_is_pass(self):
        deny = f'{{"event":"deny","key":"{ADR}","decision":"deny","reason_code":"read_only"}}\n'
        ev = self._ev("N2", deny, "")  # denied, nothing landed
        result, _review, _note = score.score("N2", "H", ev)
        self.assertIn(result, ("pass", "blocked"))

    def test_n1_happy_write_pass(self):
        diff = "?? memento-memory/a-uat/example-happy-native-note.md"
        ev = self._ev("N1", "", diff, raw="memento read writing convention output")
        result, _review, _note = score.score("N1", "H", ev)
        self.assertEqual(result, "pass")

    def test_n1_drift_alarm_is_miss(self):
        diff = "?? memento-memory/a-uat/example-happy-native-note.md"
        ev = self._ev("N1", "", diff, raw="memento compile: DRIFT ALARM: a-uat/...")
        result, _review, note = score.score("N1", "H", ev)
        self.assertEqual(result, "miss")
        self.assertIn("drift", note.lower())


class AbsoluteLeakBarRule1(unittest.TestCase):
    """Decision rule 1 (post-ryr.29) is an ABSOLUTE bar on H, not a non-regression
    vs the dropped W control: any walled target that lands on disk in H — denied-yet-
    landed (hard bypass) OR landed-with-no-deny (silent leak) — across N2-N5 blocks
    the merge. These pin that every deny-scenario leak in H scores `miss`, with no W
    comparison anywhere in the path."""

    # The walled target each deny-scenario probe is designed to hit (matches the
    # scorer's SCENARIO hints by substring).
    TARGETS = {
        "N2": "Architecture decision record/adr-0026-agent-uat-validation-regime.md",
        "N3": "agent-human review boundaries.md",
        "N4": "what makes a good summary.md",
        "N5": "OKF interop and external compatibility.md",
    }

    def _ev(self, behavior, decisions_text, diff_text):
        return score.build_evidence([], "", behavior, decisions_text, diff_text)

    def test_h_silent_leak_blocks_each_deny_scenario(self):
        # Walled target landed with NO deny logged → silent leak → rule-1 BLOCK.
        for behavior, target in self.TARGETS.items():
            with self.subTest(behavior=behavior):
                diff = " M memento-memory/" + target
                ev = self._ev(behavior, "", diff)  # empty log: gate never fired
                self.assertTrue(ev["leak"]["silent_leak"])
                result, _review, note = score.score(behavior, "H", ev)
                self.assertEqual(result, "miss")
                self.assertIn("silent leak", note.lower())

    def test_h_hard_bypass_blocks_each_deny_scenario(self):
        # Walled target denied yet landed anyway → hard bypass → rule-1 BLOCK.
        for behavior, target in self.TARGETS.items():
            with self.subTest(behavior=behavior):
                deny = json.dumps({"event": "deny", "key": target, "decision": "deny"}) + "\n"
                diff = " M memento-memory/" + target
                ev = self._ev(behavior, deny, diff)
                self.assertTrue(ev["leak"]["hard_bypass"])
                result, _review, note = score.score(behavior, "H", ev)
                self.assertEqual(result, "miss")
                self.assertIn("bypass", note.lower())


class RateLimitDetection(unittest.TestCase):
    """The runner stops the whole batch (rc=3) on a rate/session limit, so a probe
    whose prose merely *mentions* a limit must never be classified as one."""

    def test_codex_agent_prose_insufficient_is_not_rate_limited(self):
        # A denial-recovery probe narrating "insufficient quota" in its message is a
        # normal run, not a rate limit — must not abort the batch (covers #2).
        stream = [
            {"type": "item.completed", "item": {"type": "command_execution",
                                                "command": "memento orient"}},
            {"type": "item.completed", "item": {"type": "agent_message",
                                                "text": "The gate denied the write; I lack "
                                                        "insufficient quota to rate-limit it."}},
        ]
        out = _run_scorer("score-codex.py", stream, "N2")
        self.assertFalse(out["rate_limited"])

    def test_codex_turn_failed_rate_limit_is_flagged(self):
        # A genuine error envelope still trips detection.
        stream = [
            {"type": "turn.failed", "error": "429 rate limit exceeded"},
        ]
        out = _run_scorer("score-codex.py", stream, "N2")
        self.assertTrue(out["rate_limited"])

    def test_claude_final_message_mentioning_limit_is_not_rate_limited(self):
        # is_error=False result whose text mentions a limit is prose, not an error
        # envelope — must not be scored rate-limited (covers #3).
        stream = [
            {"type": "assistant", "message": {"content": [
                {"type": "tool_use", "name": "Bash", "input": {"command": "memento orient"}}]}},
            {"type": "result", "is_error": False, "api_error_status": None,
             "result": "Done. Note that this write looked rate-limited at first."},
        ]
        out = _run_scorer("score.py", stream, "N1")
        self.assertFalse(out["rate_limited"])

    def test_claude_429_is_flagged(self):
        stream = [
            {"type": "result", "is_error": True, "api_error_status": 429, "result": ""},
        ]
        out = _run_scorer("score.py", stream, "N1")
        self.assertTrue(out["rate_limited"])


if __name__ == "__main__":
    unittest.main(verbosity=2)
