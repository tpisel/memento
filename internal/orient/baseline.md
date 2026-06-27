# Memento Orientation

Memento is a thin retrieval and write-enforcement layer over a human-curated markdown memory vault. Use it to discover durable project knowledge at task start, then pull only the notes or sections that plausibly apply.

## Verbs

- `brief` prints the agent-facing manifest projection: titles, summaries, tags, headings, modes, and numeric read references.
- `read <key|@N>` reads one note or numeric brief entry; `read <key|@N>#<heading>` reads a section. When a manifest is available, it prints `binding: ratified|unratified`, `summary: current|stale|missing`, and non-empty role-flattened link lines (`inlinks:`, `outlinks:`, `transcludes:`, `transcluded-by:`) to stderr before stdout content.
  Section read: `memento read <key|@N>#<heading>` (heading text or slug from `brief`).
- `write-mode <key> <append-only|living|read-only>` durably rewrites a note's declared mode and recompiles; loosening toward `living` requires `--justification <reason>`.
- `unlock <key> --justification <reason>` records a one-off exception that re-opens a `read-only` note's edit window until the next commit. No durable mode change.
- `compile` rebuilds the manifest and derived brief artifacts from the vault.
- `init` adopts or creates a vault and installs project bootstrapping artifacts.
- `orient` prints this orientation baseline. Any project notes declaring `orient: true` in frontmatter are appended.

## How writes happen

There is no `write` verb. Author notes with your native file tools (Write/Edit/Bash on Claude, `apply_patch`/shell on codex). A PreToolUse `check-write` hook enforces the note's `mode` before the bytes land. First-draft (unratified) authoring is never walled, and new notes are created by a normal native write; modes bite only after a note's first commit. To loosen a frozen note, route the change through `write-mode` (or `unlock` for a one-off), and confirm with the user before thawing a `read-only` note.

## Write Modes

- `append-only` is the default when `mode:` is absent. Ratified notes accept appends and reject overwrites.
- `living` is for editable reference notes. Ratified notes accept appends and full-file overwrites.
- `read-only` is for frozen records such as accepted ADRs. Ratified notes reject appends and overwrites.
- Unratified notes are still in their edit window and accept appends and overwrites regardless of declared mode.

## Entry Index

Use `memento brief` when you need the doc landscape for note or section selection.
<!-- memento:brief-disclosure -->
