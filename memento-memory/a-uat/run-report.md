---
title: A-UAT run report
mode: living
tags:
  - a-uat
  - testing
  - results
summary: Append-only-in-practice results log from the A-UAT runner against the ADR-0026 test matrix. One row per model x arm x behavior x trial cell, with tool-log evidence and a provisional result. Resumable - the batch driver skips cells already recorded for the current frozen_at. Rows flagged review=yes need human adjudication. (Mode is living so the header can be restructured during the run; harden to append-only once the run set is complete.)
---

# A-UAT run report

Results from the A-UAT runner (`scripts/a-uat/run-cell.sh`, driven by `scripts/a-uat/run-batch.sh`) against [[test-matrix]] / [[adr-0026-agent-uat-validation-regime]].

## How this is maintained and resumed

- One row per `model x arm x behavior x trial` cell, appended the moment that cell completes.
- Cells are keyed by `(frozen_at, model, arm, behavior, trial)`. `run-batch.sh` reads this note and **skips any cell already recorded for the current `frozen_at`**, so a run can be stopped and resumed from any point - just re-run the driver.
- `frozen_at` is the commit the matrix was frozen at (its last-touched commit, not HEAD). A new freeze produces a new hash and a fresh result set; rows under an older hash remain that earlier run's record.
- `result` is the runner's *provisional* score from tool-log evidence (pass/miss/blocked/n-a). `review=yes` flags heuristics needing human adjudication. Full transcripts are kept under `scripts/a-uat/runs/` and named per cell.
- **Probes run blind:** each cell's worktree has this report and the matrix removed, so no probe can read its own test plan. Apparatus removal is uniform across all cells.

## Results

| frozen_at | model | arm | behavior | trial | result | review | evidence - note [status] | log |
|---|---|---|---|---|---|---|---|---|
| `325867c75218` | claude-opus | A0 | B1 | 1 | miss | no | no key tool-use (bash=3,native=0) — no orient evidence before acting [ok] | log: `scripts/a-uat/runs/20260625T203435_claude-opus_A0_B1_t1.jsonl` |
