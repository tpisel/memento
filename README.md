# memento

![CI](https://github.com/tpisel/memento/actions/workflows/ci.yml/badge.svg)

memento is a markdown-based, in-repo memory substrate for AI coding agents. It keeps durable project knowledge (decisions, constraints, discoveries) alongside the code, compiles a small manifest the agent loads at session start, and lets the agent pull only the notes or sections relevant to the current task — instead of pasting everything into `AGENTS.md`/`CLAUDE.md` every time.

The notes are markdown files. Obsidian is the authoring/browsing surface; the CLI is the agent's surface. The manifest and link graph are derived from the files, so the human view and the agent view cannot drift.

## Status

The v0–v2 surface — `init`, `compile`, `brief`, `orient`, `read`, `write` with mode enforcement, link surfaces, and auto-recompile on write — is in active dogfooding use and is the contract agents bind to. Error tokens and exit codes are stable.

Pre-1.0 means the **manifest schema may break** before 1.0 (a schema bump bumps `schema_version`; older readers refuse with `manifest-schema-unsupported`). The CLI verb surface is unlikely to break. v4 features (agent-driven summarisation worklist, `review` verb, Obsidian-open) are not built yet — see `memento-memory/spec.md` §13.

## Install

### Homebrew (macOS, recommended)

```sh
brew install tpisel/tap/memento
```

### `go install` (any Go-capable system)

```sh
go install github.com/tpisel/memento/cmd/memento@latest
```

Requires Go 1.22+. The binary lands in `$(go env GOBIN)` (or `$(go env GOPATH)/bin`); make sure that directory is on your `$PATH`.

### Pre-built binaries

Download a tarball for your OS/arch from the [latest release](https://github.com/tpisel/memento/releases/latest) and extract `memento` onto your `$PATH`.

Verify any of the above:

```sh
memento version
```

## Quickstart

From the root of a project you want to give an agent durable memory for:

```sh
# 1. Scaffold or adopt a vault. Default vault dir is <project>-memory/.
memento init

# 2. Write a note. Stdin is the body; frontmatter is optional.
echo "We rejected SQLite because the deploy target is read-only." \
  | memento write decisions/storage-choice.md

# 3. See what the agent will see at task start.
memento brief

# 4. Read a note (body to stdout; binding + links to stderr).
memento read decisions/storage-choice.md
```

`init` adds a `<!-- memento:start -->` block to `AGENTS.md`/`CLAUDE.md` (creating the file if absent) that tells the agent how to use `brief`/`read` at task start. It also writes a `.gitignore` stanza and installs a pre-commit hook that keeps `<vault>/.memento/manifest.json` in sync.

Open the vault directory itself (e.g. `my-project-memory/`) as an Obsidian vault root — *not* the repo root — so wikilinks stay bounded to the notes.

## CLI reference

- `memento help` — show help text.
- `memento version` — print the memento version.
- `memento init [--dir <vault>]` — adopt-or-create a vault and install the bootloader, pre-commit hook, and `.gitignore` stanza.
- `memento compile` — walk the vault and emit `<vault>/.memento/manifest.json` and `<vault>/_memento/brief.md`. Sub-second; safe in a pre-commit hook.
- `memento brief` — print the agent-facing markdown projection of the manifest.
- `memento orient` — print tool-usage orientation plus any project overlays.
- `memento read <key|@N>` — read a note by vault-relative key, or by the `@N` index from the brief. Supports `<key>#<heading>` for section reads. Stdout is the raw body; stderr carries `binding:` plus role-flattened link lines.
- `memento convention <name>` — read an operational convention from `_memento/conventions/<name>.md`, printing its body without frontmatter. Conventions are surfaced by `memento orient`, not the brief.
- `memento write [--overwrite] <key>` — create, append to, or overwrite a note from stdin, then auto-recompile.
- `memento write-mode <key> <append-only|living|read-only> [--justification <reason>]` — durably change a note's frontmatter mode, then auto-recompile. Loosening requires `--justification`; tightening accepts it as optional self-documentation.
- `memento unlock <key> --justification <reason>` — record a temporary single-key exception re-opening a read-only note's edit window until the next commit. The reason is held in a gitignored `.memento/unlock-grants.json` sidecar and lifted into a `Memento-Unlock:` commit trailer by the pre-commit hook.

CLI errors start with stable tokens (`unknown-command`, `invalid-arguments`, `manifest-not-found`, `manifest-schema-unsupported`, …); see `memento-memory/spec.md` for the full contract.

## Vault layout

Notes live in `<project>-memory/` by default. The machine-owned manifest lives at `<vault>/.memento/manifest.json`; tool-relevant human-readable artifacts (generated `brief.md`, optional `_memento/writing.md`, …) live in `<vault>/_memento/`. See `memento-memory/spec.md` for the full model.

## Performance

`memento compile` is expected to stay sub-second and safe for pre-commit hooks. The synthetic-vault gate is:

```sh
just bench
```

Current baseline for `BenchmarkCompile500Docs`: 18,019,817 ns/op, 22.2 MB/op, 108,096 allocs/op on Apple M2 Max, macOS arm64. The same 500-document fixture is covered by `TestCompileWithin1s`, which is skipped under `go test -short`.

## Development

If you're hacking on memento itself, invoke it in-repo via:

```sh
go run ./cmd/memento <verb>
just run <verb>
```

`just check` runs format + tests + vet + build. See `AGENTS.md` for the agent workflow used to develop this repo.

## Where to look next

- `memento-memory/spec.md` — current design specification.
- `memento-memory/Architecture decision record/` — accepted design decisions.
- `AGENTS.md` — agent workflow for this repository.
