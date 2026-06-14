# memento

![CI](https://github.com/tpisel/memento/actions/workflows/ci.yml/badge.svg)

memento is a markdown-based, in-repo memory substrate for AI agents. It keeps durable project knowledge alongside the code, compiles a small manifest for agent startup, and lets agents read only the notes or sections that are relevant to the current task.

Status: WIP / pre-1.0. CLI verbs, APIs, vault layout, and generated artifacts may change. Treat `memento-memory/spec.md` and the ADRs in `memento-memory/` as the live design surface.

## CLI

This repository is building the tool, so invoke it in-repo as:

```sh
go run ./cmd/memento <verb>
just run <verb>
```

Current verbs:

- `memento help` - show usage.
- `memento version` - print the version.
- `memento brief` - print the agent-facing manifest projection.
- `memento compile` - walk the vault and write the manifest plus generated brief.
- `memento init` - adopt or create a memory vault and wire local agent instructions.
- `memento read <key>` - read a note, or a section with `<key>#<heading>`.
- `memento write <key>` - create or append to a note from stdin.
- `memento serve` - MCP server entrypoint; currently a stub.

## Performance

`memento compile` is expected to stay sub-second and safe for pre-commit hooks. The synthetic-vault gate is:

```sh
just bench
```

Current baseline for `BenchmarkCompile500Docs`: 18,019,817 ns/op, 22.2 MB/op, 108,096 allocs/op on Apple M2 Max, macOS arm64. The same 500-document fixture is covered by `TestCompileWithin1s`, which is skipped under `go test -short`.

## Vault Layout

Notes live in a project memory directory such as `memento-memory/`. The canonical manifest is written to `<vault>/.memento/manifest.json`, and tool-relevant human-readable artifacts live under `<vault>/_memento/`. See `memento-memory/spec.md` for the full model.

## Where To Look Next

- `memento-memory/spec.md` - current design specification.
- `memento-memory/Architecture decision record/` - accepted design decisions.
- `AGENTS.md` - agent workflow for this repository.
