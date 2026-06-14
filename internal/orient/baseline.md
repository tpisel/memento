# Memento Orientation

Memento is a thin retrieval and writing layer over a human-curated markdown memory vault. Use it to discover durable project knowledge at task start, then pull only the notes or sections that plausibly apply.

## Verbs

- `brief` prints the agent-facing manifest projection: titles, summaries, tags, headings, modes, and numeric read references.
- `read <key|@N>` reads one note, numeric brief entry, or `key#heading` section. It prints `binding: ratified|unratified` to stderr before stdout content.
- `write <key>` creates or appends to a note from stdin, subject to the note's declared mode.
- `compile` rebuilds the manifest and derived brief artifacts from the vault.
- `init` adopts or creates a vault and installs project bootstrapping artifacts.
- `orient` prints this orientation baseline plus project-curated orientation notes.

## Write Modes

- `append-only` is the default when `mode:` is absent. Writes append new content; existing content is not rewritten.
- `living` is for editable reference notes. Writes may replace the file body when that operation is supported.
- `read-only` is for frozen records such as accepted ADRs. The write tool refuses to change them.

## Triggered Preconditions

None yet.

## Entry Index

Run `memento brief` to scan the entry index before deciding which notes or sections to read.
