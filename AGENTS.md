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
