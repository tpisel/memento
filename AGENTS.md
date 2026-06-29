# Agent Instructions

This project is being developed with two separate memory substrates:

- **beads** holds task state, implementation sequencing, blockers, and working notes.
- **memento memory** holds durable project knowledge: design decisions, specs, constraints, and discoveries that should survive a task.

Keep that boundary clear. Task progress belongs in beads. Durable semantic knowledge belongs in `memento-memory/`. Do not use `bd remember`.

**Context-injection discipline.** Every line memento injects into an agent's context — bootloader blocks, orient text, brief footers, error messages, and stderr metadata — must earn its place. Default to fewer words with sharper force; when adding to these surfaces, be explicit about what the line buys, and leave it out if that answer is weak.

## Start of Task

1. Run `bd ready` and choose the next ready task unless the user has already named a task.
2. Claim the selected bead before editing when the command is available and appropriate.
3. Read the selected task and any linked context before editing code.
4. Check `memento-memory/` for relevant durable context:
   - start with `memento-memory/spec.md`;
   - read ADRs linked from the task;
   - scan other ADRs when the task touches architecture, storage layout, CLI behavior, or agent workflow.
5. Keep the task loop small. Prefer finishing one beads task with tests and a close note over starting several partial threads.

When memento CLI support exists, replace the manual memory scan with the manifest/read workflow below.

<!-- memento:start -->
Durable project knowledge lives in `memento-memory`: curated design decisions, specs, constraints, and discoveries, not task state.
Before any other memento action, run `memento orient`.
Run `memento brief` when you need the doc landscape; it is pull-only, not a mandatory second step.
Use `memento read <key|@N>#<heading>` or `memento read <key|@N>` instead of grep/cat: it emits link-graph metadata on stderr and supports section extraction.
`@N` indexes come from `brief`; `memento read` writes `binding: ratified|unratified` plus non-empty role-flattened link lines to stderr before stdout content.
Discoveries that outlive a task belong in `memento-memory`, not the task store.
<!-- memento:end -->

Working state lives in beads (`bd ready`); discoveries that outlive a task go to `memento-memory/`, not beads notes.

**This repo only — invoking memento:** memento is being built in this repo. The block above refers to `memento` as if it were on `$PATH`. In this repo, invoke it as `go run ./cmd/memento <verb>` or `just run <verb>` (e.g., `just run brief`, `just run read <key>`).

## During Implementation

- Let existing code and tests define the current behavior once code exists.
- Prefer small, deterministic slices with observable CLI behavior.
- Prefer test-first development for deterministic core behavior: discovery, ignore parsing and matching, heading slugs, body hashes, manifest ordering, section extraction, and sentinel replacement.
- For CLI and integration work, write the acceptance test or fixture first when practical.
- Use `just check` for the default verification pass. Use narrower `just fmt`, `just test`, `just vet`, or `just build` commands while iterating.
- Keep implementation choices aligned with the accepted ADRs.
- Do not store transient debugging notes in `memento-memory/`.
- If a task reveals a durable constraint, rejected alternative, or design correction, add or update a memory note or ADR in the same change set.
- If the selected bead is too large or incorrectly shaped, split it or leave a bead comment with the proposed adjustment rather than forcing an oversized loop.

## End of Task

Before closing a beads task:

1. Run the relevant tests or checks.
2. Update the task with what changed, what was verified, and any remaining follow-up.
3. Move durable learnings into `memento-memory/` when they meet the writing threshold.
4. Leave beads close notes concise; do not turn them into long-term design docs.
5. Commit the bead's changes. Inside the Ralph loop wrapper, the wrapper commits for you (codex's sandbox blocks `.git/` writes anyway); do not run `git add`, `git commit`, or `git push`. Outside the loop, create one commit with first line `<bead-id>: <summary>` (e.g. `memento-2nb.3: parse .mementoignore rules`), staging explicit paths.

If a loop does not clear its selected bead, add a bead comment before stopping. Include what was attempted, what blocked progress, useful task-scoped discoveries, and exact failing commands or errors when relevant. If the discovery changes durable project understanding, also update `memento-memory/`.

## Interfaces

Use justfile for testing, linting, and formatting commands. Add them if they do not exist.

<!-- BEGIN BEADS INTEGRATION v:1 profile:minimal hash:970c3bf2 -->
## Beads Issue Tracker

This project uses **bd (beads)** for issue tracking. Run `bd prime` to see full workflow context and commands.

### Quick Reference

```bash
bd ready              # Find available work
bd show <id>          # View issue details
bd update <id> --claim  # Claim work
bd close <id>         # Complete work
```

### Rules

- Use `bd` for ALL task tracking — do NOT use TodoWrite, TaskCreate, or markdown TODO lists
- Run `bd prime` for detailed command reference and session close protocol
- Use `bd remember` for persistent knowledge — do NOT use MEMORY.md files

**Architecture in one line:** issues live in a local Dolt DB; sync uses `refs/dolt/data` on your git remote; `.beads/issues.jsonl` is a passive export. See https://github.com/gastownhall/beads/blob/main/docs/SYNC_CONCEPTS.md for details and anti-patterns.

## Agent Context Profiles

The managed Beads block is task-tracking guidance, not permission to override repository, user, or orchestrator instructions.

- **Conservative (default)**: Use `bd` for task tracking. Do not run git commits, git pushes, or Dolt remote sync unless explicitly asked. At handoff, report changed files, validation, and suggested next commands.
- **Minimal**: Keep tool instruction files as pointers to `bd prime`; use the same conservative git policy unless active instructions say otherwise.
- **Team-maintainer**: Only when the repository explicitly opts in, agents may close beads, run quality gates, commit, and push as part of session close. A current "do not commit" or "do not push" instruction still wins.

## Session Completion

This protocol applies when ending a Beads implementation workflow. It is subordinate to explicit user, repository, and orchestrator instructions.

1. **File issues for remaining work** - Create beads for anything that needs follow-up
2. **Run quality gates** (if code changed) - Tests, linters, builds
3. **Update issue status** - Close finished work, update in-progress items
4. **Handle git/sync by active profile**:
   ```bash
   # Conservative/minimal/default: report status and proposed commands; wait for approval.
   git status

   # Team-maintainer opt-in only, unless current instructions forbid it:
   git pull --rebase
   bd dolt push
   git push
   git status
   ```
5. **Hand off** - Summarize changes, validation, issue status, and any blocked sync/commit/push step

**Critical rules:**
- Explicit user or orchestrator instructions override this Beads block.
- Do not commit or push without clear authority from the active profile or the current user request.
- If a required sync or push is blocked, stop and report the exact command and error.
<!-- END BEADS INTEGRATION -->

<!-- BEGIN BEADS CODEX SETUP: generated by bd setup codex -->
## Beads Issue Tracker

Use Beads (`bd`) for durable task tracking in repositories that include it. Use the `beads` skill at `.agents/skills/beads/SKILL.md` (project install) or `~/.agents/skills/beads/SKILL.md` (global install) for Beads workflow guidance, then use the `bd` CLI for issue operations.

### Quick Reference

```bash
bd ready                # Find available work
bd show <id>            # View issue details
bd update <id> --claim  # Claim work
bd close <id>           # Complete work
bd prime                # Refresh Beads context
```

### Rules

- Use `bd` for all task tracking; do not create markdown TODO lists.
- Run `bd prime` when Beads context is missing or stale. Codex 0.129.0+ can load Beads context automatically through native hooks; use `/hooks` to inspect or toggle them.
- Keep persistent project memory in Beads via `bd remember`; do not create ad hoc memory files.

**Architecture in one line:** issues live in a local Dolt DB; sync uses `refs/dolt/data` on your git remote; `.beads/issues.jsonl` is a passive export. See https://github.com/gastownhall/beads/blob/main/docs/SYNC_CONCEPTS.md for details and anti-patterns.
<!-- END BEADS CODEX SETUP -->
