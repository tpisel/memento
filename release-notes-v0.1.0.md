# memento v0.1.0

First taggable release of memento: a markdown-based, in-repo memory substrate for AI coding agents. It keeps durable project knowledge (decisions, constraints, discoveries) alongside the code, compiles a small manifest the agent loads at session start, and lets the agent pull only the notes or sections relevant to the current task instead of pasting everything into `AGENTS.md` or `CLAUDE.md` every time. The notes stay ordinary markdown files for humans, while the CLI is the stable agent surface.

## What's in v0.1.0

The v0-v2 CLI surface is in active dogfooding use:

- `init` scaffolds or adopts a marker-based vault, installs the bootloader guidance, and keeps the manifest wired into the repo workflow.
- `compile` derives `.memento/manifest.json` plus the generated `_memento/brief.md` projection from markdown notes, headings, tags, summaries, and links.
- `brief` and `orient` give agents a compact start-of-task map of the vault and the local operating rules.
- `read` supports whole-note, `@N`, and `#heading` reads, with binding and role-flattened link surfaces on stderr.
- `write` creates, appends to, or overwrites notes according to mode enforcement, then auto-recompiles after a successful write.

CLI error tokens and exit codes are stable, including `manifest-schema-unsupported`. The current manifest schema is v1.

## Install

```sh
brew install tpisel/tap/memento
# or
go install github.com/tpisel/memento/cmd/memento@latest
```

See the [README quickstart](https://github.com/tpisel/memento#quickstart) for the 5-minute walkthrough after install.

## Pre-1.0 caveat

The manifest schema may break before 1.0. A schema bump updates `schema_version`, and older readers refuse the manifest with `manifest-schema-unsupported` instead of reading it incorrectly. The CLI verb surface and error tokens are intended to stay stable.

## Not yet

Agent-driven summarisation worklists, the `review` verb, standalone CLI auto-summarisation, and an Obsidian-open helper are planned for v0.2+ work. They are not part of v0.1.0.
