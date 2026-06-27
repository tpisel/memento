---
title: codex-cli lifecycle hooks contract
summary: "Empirical pin (codex-cli 0.142.2) of the lifecycle-hooks config.toml schema, the PreToolUse wire contract, the trust-by-hash flow, and ask support. CRITICAL (memento-ryr.31): codex declares hooks INLINE only — `hooks` must be a HooksToml struct ([[hooks.<Event>]] array-of-tables); the path-indirection form `hooks = \"hooks.json\"` is REJECTED at config load (`invalid type: string, expected struct HooksToml`), so memento ships no hooks.json. PreToolUse permissionDecision is {allow,deny,ask} byte-identical to Claude Code (so vault_discovery_ambiguous CAN use ask, no deny-fallback needed); deny requires hookEventName in hookSpecificOutput; trust is persisted per-hash (trusted_hash) and bypassable only via --dangerously-bypass-hook-trust. Confirms ADR-0031's codex-in-scope premise. Also flags a side discovery: memento's own wikilink extractor matches double-bracket tokens inside code spans/fences."
tags:
  - memento
  - codex
  - hooks
  - enforcement
  - agents
  - spike
mode: living
status: reference
date: 2026-06-27
---

# codex-cli lifecycle hooks contract

Spike for ADR-0031 (memento-ryr.15) to pin the empirical facts the codex install
beads depend on. Evidence is `codex-cli 0.142.2` itself, not docs: `codex features
list` (feature flag state), the JSON-Schema blobs the binary embeds for every hook
event's stdin/stdout (`strings` over the Mach-O), the embedded config reference, and
a `codex doctor` parse test against a hand-written `config.toml`. Nothing here is
inferred from Claude Code — it is read off codex's own wire schema, which happens to
match.

## Feature is real and stable

`codex features list` reports **`hooks  stable  true`**. (`plugin_hooks` is `removed`;
`request_permissions_tool`/`exec_permission_approvals` are still under development —
those are a different surface.) So lifecycle hooks are on by default in ≥ 0.142, not
an experimental opt-in. This retires ADR-0025's "codex = skill-only, no hooks".

## Lifecycle events

Nine events ship (`HookEventsToml`): **PreToolUse, PermissionRequest, PostToolUse,
PreCompact, PostCompact, SessionStart, UserPromptSubmit, SubagentStart,
SubagentStop**. Each event's stdin/stdout is JSON, one object per line, with an
embedded draft-07 schema — i.e. the **JSON-on-stdin → JSON-on-stdout, synchronous
shell command** contract ADR-0031 assumed.

`memento` only needs **PreToolUse** (the pre-mutate gate, Claude-equivalent) and
**PostToolUse** (compile + drift alarm). `PermissionRequest` is a codex-only extra
(see caveat below) we do not use.

## PreToolUse — the gate `check-write` answers

**Input** (`pre-tool-use.command.input`), required keys:
`cwd, hook_event_name, model, permission_mode, session_id, tool_input, tool_name,
tool_use_id, transcript_path, turn_id` (plus optional `agent_id`, `agent_type`).
`tool_input` is untyped (`true` in schema) — the raw tool payload, same role as
Claude's `tool_input`. Codex adds `turn_id`/`model`/`permission_mode`/`tool_use_id`;
`hook_event_name` const is `"PreToolUse"`. `permission_mode` enum:
`default | acceptEdits | plan | dontAsk | bypassPermissions`.

**Output** (`pre-tool-use.command.output`) — two accepted forms:

- **Claude-shaped (use this):**
  ```json
  {"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"<text>"}}
  ```
  `permissionDecision` enum (`PreToolUsePermissionDecisionWire`) is
  **`allow | deny | ask`**. `hookEventName` is a **required** field of
  `hookSpecificOutput` — the ADR's shorthand
  (`{"hookSpecificOutput":{"permissionDecision":"deny",…}}`) omits it, but codex's
  schema requires it (Claude also expects it), so the byte-identity claim only holds
  with `hookEventName` present. This is exactly what `check-write` already emits — see
  [[check-write output contract]] — so the same stdout works on both harnesses.
- **Legacy top-level:** `decision` (`PreToolUseDecisionWire` = `approve | block`) +
  `reason`. We do not emit this; noted so a stray `decision` key is read correctly.

Extra top-level fields exist (`continue`, `stopReason`, `suppressOutput`,
`systemMessage`); all default to no-op. The schema is `additionalProperties:false`
at the top level — but `check-write`'s extra `reason_code` rides **inside** behaviour
that the harness ignores in practice (verify on first live-fire; if codex rejects
unknown top-level keys, `reason_code` must move or be dropped for codex).

## ask IS supported (resolves the vault_discovery_ambiguous fallback)

`permissionDecision: "ask"` is a first-class PreToolUse value in codex. So
ADR-0031's `vault_discovery_ambiguous` row — "ask (not deny): set
MEMENTO_VAULT_ROOT" — works natively on codex; **no deny-fallback is required.**
(`PermissionRequest`'s decision is allow/deny only — but that is not the event
`check-write` answers.)

## PostToolUse

Output `decision` is `BlockDecisionWire` = **`block` only** (no allow — PostToolUse
runs after the mutation). `hookSpecificOutput` carries `additionalContext` /
`updatedMCPToolOutput`. Matches Claude. compile's drift alarm can surface via
`block` + `reason`/stderr.

## Exit-code semantics match Claude

The binary carries messages like *"PostToolUse hook exited with code 2 but did not
write feedback to stderr"* and *"PermissionRequest hook exited with code 2 but did
not write a denial reason to stderr"*. So **exit 2 + stderr = block/deny**, same
convention ADR-0031 relies on ("harness blocks only on exit 2 or explicit
permissionDecision:deny"). Other non-zero exits are not the block path.

## Config: where hooks are declared

> Note: TOML array-of-tables headers are shown here with spaced brackets
> (`[ [hooks.PreToolUse] ]`) **only** to dodge memento's own wikilink extractor —
> see the parser caveat at the end. Real codex TOML uses the tight `[[...]]` form.

- `config.toml` has a top-level **`hooks`** field (`ConfigToml`, alongside
  `skills`/`plugins`/`marketplaces`). Two shapes:
  - **inline TOML**, keyed by PascalCase event name. A PreToolUse command hook is an
    array-of-tables entry under header `hooks.PreToolUse` (`matcher`, then a nested
    array-of-tables `hooks.PreToolUse.hooks` whose entries are
    `type = "command"`, `command = "memento check-write"`, `timeout_sec = 5`).
  - **path indirection — DOES NOT WORK at runtime (memento-ryr.31).** The embedded
    schema carries a `HooksFile` variant suggesting `hooks = "./hooks.json"`, but
    codex-cli 0.142.2 **rejects a string value for `hooks`** at config load:
    `invalid type: string "hooks.json", expected struct HooksToml` (reproduced under
    `codex exec --strict-config`). So `hooks` must be an inline `HooksToml` struct;
    the path form is not loadable. memento therefore emits the **inline** form only
    and ships no `hooks.json`. (The old b16 install used the path form and was wholly
    rejected — every codex A-UAT cell errored at config load before running; see the
    2026-06-27 void run.) The JSON shape below is kept only as a shape reference for
    the matcher-group/handler fields; it is **not** a valid `config.toml` value:
    ```json
    {
      "PreToolUse": [
        {
          "matcher": "Shell",
          "hooks": [
            { "type": "command", "command": "memento check-write", "timeout_sec": 5 }
          ]
        }
      ]
    }
    ```
- `MatcherGroup = { matcher, hooks: [HookHandlerConfig] }`; the handler enum is
  internally tagged on `type` with `Command | Prompt | Agent`. The `Command` variant
  fields (from the app-server `ConfiguredHookHandler` type): `command`,
  `commandWindows`, `timeoutSec`, `async`, `type` (TOML authoring keys are the
  snake_case forms, e.g. `timeout_sec`, `command_windows`).
- **`init` install note (b16):** install PreToolUse + PostToolUse entries; keep each
  command a dumb pipe to `memento check-write` / `memento compile`.
- **b16 realized (memento-ryr.16), corrected by memento-ryr.31.** `init` branches per
  family: Claude is the always-installed baseline; codex is additive, gated on a
  `.codex/` dir in the repo. The codex install writes **project-local `.codex/`** with
  the SessionStart/PreToolUse/PostToolUse hooks declared **inline** in `config.toml`
  inside a memento sentinel block — `[[hooks.<Event>]]` array-of-tables, each with a
  `matcher` and a nested `[[hooks.<Event>.hooks]]` command handler pointing at a
  `.codex/memento-*.sh` copy of the same dumb-pipe script Claude uses (byte-identical,
  pinned by a drift test). **There is no `.codex/hooks.json`** — codex rejects the
  path-indirection form (above). PreToolUse/PostToolUse matcher is the broad
  `apply_patch|Shell` (codex tool_name strings are unpinned — over-firing is harmless).
  Each handler carries `timeout_sec` (gate 5, compile 30). The block is **appended at
  the end** of config.toml: array-of-tables headers must follow every top-level key,
  and appending leaves nothing for them to capture (the b25/b27 top-insertion +
  bracket-depth machinery existed only for the bare `hooks = …` key and was removed).
  Two degradations preserve the additive invariant: a foreign `hooks` key/table
  already in `config.toml` is left untouched with a manual-wiring notice (no TOML dep,
  so this is a line heuristic, not a parse); and the **hook-trust step is surfaced on
  stdout** (`InitOptions.NoticeWriter`) — codex installs the hook untrusted, so the
  gate is fail-open until the user trusts it by hash or passes
  `--dangerously-bypass-hook-trust`.
- **OPEN — does codex read project-local `.codex/config.toml`?** This spike only
  confirmed the config.toml *schema* against a hand-written file via `codex doctor`,
  never *where* codex loads it from (`CODEX_HOME` defaults to `~/.codex`). If codex
  only reads the global `~/.codex/config.toml`, the b16 project-local install is a
  silent fail-open — the gate never fires. Verify the load path at the A-UAT live-fire
  (ADR-0026); if global-only, `init` must also wire (or instruct wiring of) the global
  config, and `doctor` liveness must check it.
- **Caveat — doctor does NOT deep-validate hooks.** With a hand-written
  `config.toml`, `codex doctor` reports `config.toml parse  ok` even for an unknown
  event name or an unknown handler field. "parse ok" = valid TOML, **not** valid hook
  config. memento's own `doctor` liveness check cannot lean on codex doctor to confirm
  the gate is wired; it must check the hook shape itself (consistent with ADR-0031
  making doctor a hard dependency).

## Trust model — trust-by-hash, so init can't silently install a live gate

Confirmed. Hook trust is **persisted per content hash**: `ProjectConfig`/
`HookStateToml` carry a **`trusted_hash`**; the binary has a TUI trust flow
(`hooks_browser_view.rs`, *"Failed to trust hook"*, *"is marked as untrusted in …"*)
and an override flag **`--dangerously-bypass-hook-trust`** (env `BYPASS_HOOK_TRUST`,
described "Run enabled hooks without requiring persisted hook trust for this
invocation. DANGEROUS. Intended only for automation that already vets hook sources").

Consequence for `init`: writing the `config.toml`/`hooks.json` entry installs the
hook **untrusted** — it is skipped until the user reviews and trusts it (its hash is
recorded). `init` therefore **cannot install a live gate non-interactively**; it can
only stage the config and must tell the user to trust it (or the run must opt into
the dangerous bypass). This is the codex analogue of the fail-open-on-absence honesty
in ADR-0031: on codex the gate is additionally fail-open-until-trusted.

## apply_patch envelope parsing in check-write (memento-ryr.9)

How `check-write` consumes a codex `apply_patch` PreToolUse call, given this spike
left `tool_input`'s exact shape unpinned (it is untyped — schema `true`):

- **Envelope recovery is key-agnostic.** `check-write` does not bet on a key name
  (`input`? `patch`?) or even on `tool_input` being an object — it scans the raw
  `tool_input` for the first string value, at any depth or as a bare string, that
  contains the `*** Begin Patch` marker. An `apply_patch` call whose payload yields
  no recognisable envelope **fails closed** (exit non-zero), since its targets
  cannot be determined. This is the safe resolution of the "exact key" live-fire
  unknown; confirm the real key on first live-fire but no code change is needed if
  it differs.
- **Multi-section patches gate per file, deny-on-first.** An envelope may carry
  several file sections; each that resolves into the vault is gated against the same
  invariant/drive-by/verdict path as a Claude write, and the **whole call is denied
  on the first violation** (the tool call is atomic). A patch touching no vault note
  is inert. Only `Update` sections seed the drift ledger (hunks applied to known
  disk bytes are exact); an `Add` is allowed as creation regardless of exact bytes,
  so it is not recorded, to avoid a false drift alarm.
- **Update/Add derive bytes; Delete and rename are deferred.** `Update File` replays
  hunks against disk-old (`enforce.ApplyHunks`); `Add File` yields the added lines.
  A `Delete File` or an `Update File` with `*** Move to:` that touches a vault note
  is **denied** (`reason_code: apply_patch_unsupported_op`) — these change a note's
  existence/identity, not its body, so they are held back fail-closed until a verb
  owns that operation rather than gated on bytes we cannot model.

## Limits of this spike

- The PreToolUse **deny was confirmed against codex's own embedded schema**, not by a
  live tool call — this box's codex is unauthenticated (`doctor` handshake → HTTP 401),
  so an end-to-end "deny actually blocks an `apply_patch`/shell call" run is deferred
  to the A-UAT gate (ADR-0026). The schema is authoritative for the contract shape;
  execution-level byte-identity is schema-confirmed, not yet run-confirmed.
- `reason_code` survival as a top-level extra on codex (vs `additionalProperties:false`)
  is the one thing to check on first live-fire.

## Side discovery — memento's wikilink extractor ignores code context

Authoring this note surfaced a memento parser limitation worth recording: the
double-square-bracket wikilink extractor matches **inside fenced code blocks and
inline code spans**, not just prose. TOML array-of-tables headers written tight
(double-`[` ... double-`]`) inside a ```` ```toml ```` fence, and the same token in
an inline code span, both became dangling outlinks (e.g. `hooks.PreToolUse`,
`hooks.PreToolUse.hooks`) on `compile`/`read`. Any note documenting TOML, Obsidian,
or other syntax that uses the double-bracket token will silently pollute the link
graph. Worth a separate bead to make link extraction skip code spans/fences; until
then, escape or space the brackets as done above.

## Related

- [[Architecture decision record/adr-0031-remove-write-verb-hook-enforced-native-writes]]
  — this spike pins its "Multi-agent" §; confirms codex-in-scope, corrects the deny
  envelope to require `hookEventName`, and resolves `vault_discovery_ambiguous` to a
  native `ask`.
- [[check-write output contract]] — the stdout `check-write` already emits; verified
  here to be codex-PreToolUse-valid as-is.
- [[adr-0026-agent-uat-validation-regime]] — owns the live-fire A/B gate that closes
  the execution-confirmation gap above.
