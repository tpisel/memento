---
title: A-UAT run report
mode: append-only
tags:
  - a-uat
  - testing
  - results
summary: Append-only log of A-UAT cells run against the ADR-0026 test matrix. One row per model x arm x behavior x trial, with tool-log evidence and a provisional result. Rows flagged review=yes need human adjudication; never overwrite, only append.
---

# A-UAT run report

Append-only results from the A-UAT runner (`scripts/a-uat/run-cell.sh`) against [[test-matrix]] / [[adr-0026-agent-uat-validation-regime]]. Each row is one `model x arm x behavior x trial` cell. `result` is the runner's provisional score from tool-log evidence; `review=yes` marks heuristics needing human adjudication. Full transcripts are kept under `scripts/a-uat/runs/`.

| frozen_at | model | arm | behavior | trial | result | review | evidence — note [status] | log |
|---|---|---|---|---|---|---|---|---|
