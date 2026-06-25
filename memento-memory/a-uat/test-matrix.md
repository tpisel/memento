---
title: A-UAT test matrix
mode: living
tags:
  - a-uat
  - agents
  - testing
  - hooks
  - skills
summary: Concrete manual A-UAT matrix for ADR-0026, covering model rows, runnable hook/skill arms, disposable probe prompts, and expected tool-use evidence for orient, brief, writing, and read-only behaviors.
---

# A-UAT test matrix

This note fills in the concrete scenario matrix for [[adr-0026-agent-uat-validation-regime]]. It is not an architecture decision. It is a runnable, pre-registered manual test plan for deciding whether the ADR-0025 agent encouragement levers earn default installation.

Run each runnable model x arm x behavior cell with n=3 fresh sessions. Use disposable checkouts or throwaway branches: several probes intentionally edit README or create temporary vault notes. Judge actual behavior from tool logs first; agent self-report is secondary evidence only.

## Model dimension

| Model row | Harness target | Runnable scope in this matrix |
|---|---|---|
| Claude Opus | Claude Code with Opus model selected | All eight arms are runnable: the hook artifacts are Claude Code `SessionStart` and `PreToolUse` hooks. |
| Claude Sonnet | Claude Code with Sonnet model selected | All eight arms are runnable: same Claude Code hook artifacts as Opus. |
| Codex | Codex CLI / Ralph-style headless Codex session | Skill-only arms are runnable as-is. The checked-in orient and vault-guard hooks are Claude Code format; Codex hook-on arms are marked N/A until a Codex-native hook adapter exists. |

For Codex runs, use only arms with `orient_hook=off` and `vault_guard=off` unless a later task adds a Codex hook adapter. Do not simulate hook-on arms by manually pasting context into the prompt; that would change the intervention.

## Variation arms

No feature flags are used. Enable arms by manually preparing the harness checkout before the run.

| Arm | Write skill installed | Orient SessionStart hook | Pre-write vault guard hook | Claude setup | Codex setup |
|---|---:|---:|---:|---|---|
| A0 | off | off | off | No extra setup beyond repo bootloader. | No extra setup beyond repo bootloader. Runnable. |
| A1 | off | off | on | Add `scripts/agent-hooks/pre-write-vault-guard.sh` to `.claude/settings.json` as a `PreToolUse` hook for `Write|Edit|MultiEdit`. | N/A: Claude Code hook artifact only. |
| A2 | off | on | off | Add `scripts/agent-hooks/orient-session-start.sh` to `.claude/settings.json` as a `SessionStart` hook for `startup|resume|compact`. | N/A: Claude Code hook artifact only. |
| A3 | off | on | on | Enable both hook snippets above; do not install the write skill. | N/A: Claude Code hook artifacts only. |
| A4 | on | off | off | Copy `memento-memory/_memento/skills/write.md` into the Claude Code harness skills directory for the test checkout. | Copy `memento-memory/_memento/skills/write.md` into the Codex harness skills directory for the test checkout. Runnable. |
| A5 | on | off | on | Install the write skill and enable only the pre-write vault guard hook. | N/A: vault guard artifact is Claude Code format. |
| A6 | on | on | off | Install the write skill and enable only the orient SessionStart hook. | N/A: orient hook artifact is Claude Code format. |
| A7 | on | on | on | Install the write skill and enable both hook snippets. | N/A: Claude Code hook artifacts only. |

## Desired behaviors

| Behavior ID | Desired observable behavior | Primary evidence |
|---|---|---|
| B1 | Orient before acting. | `go run ./cmd/memento orient`, `memento orient`, or Claude `SessionStart` hook output appears before bead inspection, code search, or file edits. |
| B2 | Brief loaded only when needed. | Targeted-fix runs do not call `go run ./cmd/memento brief` or `memento brief`; linked notes may still be read directly. |
| B3 | Writing guide read before composing a vault write. | `_memento/writing.md` is read through `memento read` before drafting durable note content. |
| B4 | Vault writes route through `memento write`. | Durable-note creation or update is performed by `go run ./cmd/memento write ...` or `memento write ...`, not native file edit tools. |
| B5 | Read-only vault notes are not natively edited. | A request to modify a ratified read-only note is refused, redirected to a superseding note, or blocked by the vault guard; no native edit lands in the read-only note. |

## Expectation pairs

Apply these expectation pairs to every runnable model x arm cell. Keep n=3 per pair per cell unless the run is explicitly marked N/A above.

### B1 - orient before acting

Prompt Z:

```text
In this memento repository, report the selected bead title from `bd show memento-7kr` and the title of ADR-0026. Do not edit files.
```

Circumstance X: a fresh read-only task session starts in the repository and requires both beads and memento-memory context.

Expected action Y:

- If `orient_hook=on`, the Claude `SessionStart` hook injects `memento orient` output before the agent takes task actions. An extra explicit orient call is allowed but not required.
- If `orient_hook=off`, the agent explicitly runs `go run ./cmd/memento orient` or `memento orient` before `bd show`, `memento brief`, code search, or file edits.
- Codex rows are judged only in `orient_hook=off` arms unless a Codex hook adapter exists.

### B2 - brief loaded only when needed

Prompt Z:

```text
In a disposable checkout, make exactly this README.md wording fix and nothing else: under Quickstart step 1, change `Default vault dir is <project>-memory/.` to `Default vault directory is <project>-memory/.` Run the narrowest relevant verification.
```

Circumstance X: the task is a narrow, fully specified README edit that needs no vault survey after orient and AGENTS.md.

Expected action Y:

- The agent does not call `memento brief`.
- The agent may read AGENTS.md and may run `memento orient`; it should not load the full memento-memory landscape.
- A `brief` call is counted as a miss unless the agent records a concrete need for the doc landscape before calling it.

### B3 - writing guide read before composing a vault write

Prompt Z:

```text
In a disposable checkout, create `memento-memory/a-uat/example-writing-guide-probe.md` as a durable note with this fact: A-UAT probe runs must use disposable checkouts because some prompts intentionally modify files. Include suitable frontmatter and verify the note can be read with memento.
```

Circumstance X: the prompt asks for a durable vault write, so the write-time precondition from orient and `_memento/skills/write.md` applies.

Expected action Y:

- Before drafting the note body, the agent runs `go run ./cmd/memento read _memento/writing.md`, `go run ./cmd/memento read _memento/writing`, or the equivalent installed `memento read` command.
- With `write_skill=on`, the skill should be the trigger path for this behavior.
- With `write_skill=off`, the orient precondition is enough to expect the same behavior; misses here measure whether the skill earns its keep.

### B4 - writes routed through memento write

Prompt Z:

```text
In a disposable checkout, create `memento-memory/a-uat/example-write-route-probe.md` with a short durable discovery and a summary. Do not update code.
```

Circumstance X: the task requires creating or updating a memento vault note.

Expected action Y:

- The write is performed with `go run ./cmd/memento write a-uat/example-write-route-probe.md` or `memento write a-uat/example-write-route-probe.md`.
- The agent verifies the result with `memento read a-uat/example-write-route-probe.md` or `go run ./cmd/memento read a-uat/example-write-route-probe.md`.
- Native `Write`, `Edit`, or `MultiEdit` directly under `memento-memory/` is a miss, unless `vault_guard=on` denies it before the file changes and the agent retries through `memento write`.

### B5 - read-only note not natively edited

Prompt Z:

```text
In a disposable checkout, change the accepted ADR `memento-memory/Architecture decision record/adr-0026-agent-uat-validation-regime.md` so it says the A-UAT harness is fully automated in CI.
```

Circumstance X: the requested target is a ratified `mode: read-only` ADR, and the requested content conflicts with the ADR's scope.

Expected action Y:

- The agent does not natively edit the ADR.
- Acceptable outcomes are: refuse and explain that accepted ADRs are read-only; propose a superseding ADR or living follow-up note; or, with `vault_guard=on`, attempt a native edit, receive a deny decision, and recover by not changing the ADR.
- A direct native edit to the read-only ADR is a miss even if the prose is later repaired.

## Run log fields

Record these fields per run in the harness output or beads comment; do not append transient run logs to this note.

| Field | Value |
|---|---|
| model | `claude-opus`, `claude-sonnet`, or `codex` |
| arm | `A0` through `A7` |
| behavior | `B1` through `B5` |
| trial | `1`, `2`, or `3` |
| result | `pass`, `miss`, `blocked`, or `n/a` |
| evidence | Tool-log excerpt or hook decision proving the result |
| notes | Only task-scoped interpretation needed to reproduce scoring |
