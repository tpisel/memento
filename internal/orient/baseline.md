# Memento Orientation

Memento is a thin retrieval and writing layer over a human-curated markdown memory vault. Use it to discover durable project knowledge at task start, then pull only the notes or sections that plausibly apply.

## Verbs

- `brief` prints the agent-facing manifest projection: titles, summaries, tags, headings, modes, and numeric read references.
- `read <key|@N>` reads one note, numeric brief entry, or `key#heading` section. It prints `binding: ratified|unratified` and non-empty role-flattened link lines (`inlinks:`, `outlinks:`, `transcludes:`, `transcluded-by:`) to stderr before stdout content.
- `write [--overwrite] <key>` creates, appends to, or overwrites a note from stdin, subject to the note's declared mode, then recompiles the vault.
- `compile` rebuilds the manifest and derived brief artifacts from the vault.
- `init` adopts or creates a vault and installs project bootstrapping artifacts.
- `orient` prints this orientation baseline plus project-curated orientation notes.

## Write Modes

- `append-only` is the default when `mode:` is absent. Ratified notes accept appends and reject overwrites.
- `living` is for editable reference notes. Ratified notes accept appends and full-file overwrites.
- `read-only` is for frozen records such as accepted ADRs. Ratified notes reject appends and overwrites.
- Unratified notes are still in their edit window and accept appends and overwrites regardless of declared mode.

## Triggered Preconditions

None yet.

## Entry Index

Run `memento brief` to scan the entry index before deciding which notes or sections to read.
