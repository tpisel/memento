---
title: "memento doctor — scope, the two-axis check model, and the diagnose-only contract"
status: proposal
mode: living
date: 2026-06-30
tags:
  - memento
  - doctor
  - enforcement
  - health
  - cli
  - adr
summary: "Defines memento doctor as a single check engine over a precondition DAG, diagnose-only in v1 (--fix deferred). The discriminator: every check output is a property of the installation or a derived artifact, never of a note — which ejects broken links, orphans, frontmatter validity, and 'pulling its weight' to compile/review. Each check carries two ORTHOGONAL axes: class (liveness vs hygiene — the faculty-region) and assertable-in {session,ci,any} (where it may gate), because they cross (core.hooksPath shadowing is liveness-class yet CI-assertable; BYO-conformer presence is hygiene-class yet fine-when-absent on a fresh runner). Severity is ok/nudge/warning/error plus skip-with-reason as a first-class exit-neutral outcome. Exit spine stays 0=clean / 1=broken / 2=usage; warnings exit 0 by default and only gate under MEMENTO_DOCTOR_STRICT (mirrors MEMENTO_STRICT_COMMIT). Liveness is behavioral not byte-match: the pre-commit anchor is checked for core.hooksPath/husky shadowing, not script identity. Manifest freshness is an authoritative side-effect-free in-buffer recompile+diff that shares brief's predicate at higher fidelity. Subsumes the liveness verb shipped in memento-aan/mbd, satisfies ADR-0031's named hard dependency, and amends ADR-0029's convention-check routing."
---

# ADR-0032 — `memento doctor`: scope, the two-axis check model, and the diagnose-only contract

> **Status: proposal.** This note is `living` while under review and flips to
> `read-only` on acceptance, as ADR-0031 did. It expands `doctor` beyond the v1
> liveness pass that already shipped (memento-aan, memento-mbd) and converges the
> candidate checks accreted in [[doctor-scoping]] into a committed scope and contract.

## Decision

`memento doctor` is a **single check engine** evaluating a **precondition DAG** of
mechanical checks over a memento installation. It is **diagnose-only in v1**: it
prints each finding and the remediation command but runs no fix and writes nothing
(`--fix` is deferred). It runs **all** reachable checks every invocation —
dependency-ordered, never short-circuiting — and **skips with a reason** any check
whose precondition failed.

Four decisions give the engine its shape:

1. **The discriminator** — every check's output is a property of the *installation*
   or a *derived artifact*, **never a property of a note**. This is sharper than
   "closed-world mechanical" (broken wikilinks pass *that* and still must be out).
2. **Two orthogonal axes per check** — a **class** (`liveness` | `hygiene`: the
   faculty-region the check lives in) and an **assertability-context**
   (`assertable-in: {session, ci, any}`: where the check is allowed to *gate*).
   They are independent because they genuinely cross.
3. **Severity is `ok` / `nudge` / `warning` / `error`, plus `skip`** — `skip` is a
   first-class, exit-neutral-but-not-green outcome, not a green pass in disguise.
4. **Exit contract reuses the in-tree strict idiom** — the `0/1/2` spine is
   preserved (`2` is already "usage error" CLI-wide); warnings exit `0` by default
   and gate only under `MEMENTO_DOCTOR_STRICT` (the `MEMENTO_STRICT_COMMIT` pattern).

This **subsumes** the liveness checks shipped in the v1 verb: doctor is the whole
engine, and the SessionStart orient hook (memento-mbd) simply *invokes* it. The
three cadences of [[doctor-scoping]] (liveness/SessionStart, ratification/compile,
hygiene/occasional) are **invocation contexts**, not three verbs — the context is
expressed by the assertability-context mask, not by re-splitting the surface.

## What this supersedes / amends

- **[[doctor-scoping]] — its single-verb framing is replaced.** That note worried
  doctor was "being asked to span three cadences." The resolution is not three
  verbs: it is one engine whose checks carry an `assertable-in` mask, so the same
  binary serves SessionStart, CI, and manual cadences. The candidate-check inbox
  there is now committed scope here.
- **[[adr-0031-remove-write-verb-hook-enforced-native-writes]] — its named hard
  dependency is satisfied.** ADR-0031 calls doctor "the only loud surface for *is
  enforcement actually on*" and defers it to "its own ADR." This is that ADR. The
  shipped v1 (liveness-only) becomes the `liveness`-class subset of this engine.
- **[[adr-0029-convention-files-and-convention-verb]] — its convention-check
  routing is split, not deleted.** ADR-0029 routed "malformed conventions"
  (disallowed frontmatter keys; missing/empty `when_to_read:`) *to* doctor. Those
  are properties of a *note*, so the discriminator ejects them — but it also reveals
  that ADR-0029 conflated two different failures (see *Out of scope* below).
- **[[adr-0010-tool-read-writing-guide]] — its deferred "missing `writing.md`"
  signal lands here as a `nudge`.** init scaffolds `_memento/conventions/writing.md`,
  so by the init↔doctor symmetry its *presence* is doctor's business; its *quality*
  is not.

## Scope

### The discriminator

> **Every doctor check reports a property of the installation or a derived artifact.
> No doctor check reports a property of a note.**

doctor may *consume* notes to recompute derived state (manifest freshness reads
every note), but it never passes judgment on an individual note. "Closed-world
mechanical" is too weak a line — a broken-wikilink scan is mechanical and
closed-world yet judges a note, so it must be out. The property-of-what test is the
discriminator that actually holds.

Two edges to pin so the line does not wobble:

- **Manifest freshness is the load-bearing edge case.** It consumes every note and
  may *cite* one as evidence ("`foo.md` is newer than the manifest"). Citing a note
  as a timestamp/hash fact is **not judging** it — the output is a property of the
  *artifact* ("the manifest is stale"), not a quality verdict on `foo.md`. Freshness
  is therefore in scope.
- **The grant-staleness check is a deliberate boundary case.** It reads `git status`
  to decide whether an unlock-sidecar entry has a matching uncommitted edit. It
  judges a *grant entry* (installation state), not a note, so it stays in — but it is
  the one check that reaches into the working tree, and that is called out, not
  hidden.

Keeping note-judgment out is what preserves the **layer signal**: a green doctor
means *the machine works*, independent of how tidy the corpus is. The moment doctor
opines on corpus quality, "doctor is green" stops meaning "enforcement and the
pipeline are live."

### In scope

- **Hook liveness** — the PreToolUse `check-write` gate and the PostToolUse compile
  hook are wired for each detected agent family, command resolves and is executable,
  matcher covers the write tools (shipped in v1).
- **Pre-commit anchor liveness** — the git pre-commit hook memento installs is the
  hook git will *actually run* (see *Liveness ≠ presence*).
- **Manifest freshness** — the on-disk manifest matches an authoritative in-buffer
  recompile (see *Manifest freshness*).
- **Manifest schema *read* compatibility** — this binary can decode the on-disk
  manifest (`schema_version ≤ CurrentSchemaVersion`). Distinct from `compile`'s
  *write-time* validity, and split from the liveness framing (see *Schema-compat*).
- **Config validity** — `.memento/config.toml` parses and its keys are recognised.
- **Vault discoverability** — exactly one marker dir resolves from the repo root
  (ambiguity is reported, never a panic).
- **Ignore correctness** — the `.gitignore` memento stanzas (and `.mementoignore`)
  are present and well-formed.
- **Presence of expected tool-read files** — bootloader / orient / convention
  scaffolding init establishes (presence only; `writing.md` absent is a `nudge`).
- **Presence-and-runnability of a declared BYO conformer** — that a configured
  external conformer resolves and is executable. **Running it is out** (its output
  judges notes — that is `review`'s).

### Out of scope (named, with where they re-home)

These are deliberately excluded to keep the layer signal; none is *deleted*, each
re-homes by kind:

- **Broken wikilinks, orphans, markdown conformance, "pulling its weight"** →
  `review` / `compile`. All judge a note.
- **Disallowed frontmatter key** → **`compile`**, plausibly a *hard* error: the note
  will not project cleanly into the manifest.
- **Missing / empty `when_to_read:`** → **`review`**, an advisory *convention*
  finding: the note projects fine, it is just under-specified.

The last two are the ADR-0029 split. ADR-0029 was not simply "wrong to route to
doctor" — it **conflated a hard-structural failure with an advisory-convention one**
and sent both to doctor. The discriminator separates them: structure to `compile`,
taste to `review`.

## The two-axis check model

Each check declares **two independent attributes**:

- **`class`** — the faculty-region: `liveness` (a property of *this machine / this
  clone*: is enforcement actually on *here*) vs `hygiene` (a property of the
  *committed vault / installation*: is the configuration well-formed).
- **`assertable-in`** — a subset of `{session, ci, any}`: the contexts in which the
  check is permitted to **gate** (contribute to the exit code). A check still *runs*
  and *reports* everywhere; the mask governs only whether a failure counts against
  the caller's exit code in that context.

**They are two axes because they cross.** Collapsing them into one tag forces an
exception list within months. The crossings that prove it:

- **`core.hooksPath` shadowing** is `class: liveness` (per-clone machine state) yet
  **CI-assertable** — CI clones the repo and has a git config, so it absolutely can
  detect that the committed pre-commit anchor is shadowed. `assertable-in:
  {session, ci}`.
- **BYO-conformer presence** is `class: hygiene` (a committed-config fact) yet on a
  fresh CI runner "not installed" is often *fine and expected*, so it should not gate
  CI. `assertable-in: {session}` (or `nudge` everywhere).
- **The live-fire self-test** is `class: liveness` (it proves the verdict chain
  bites) yet **hermetic** — it builds a throwaway temp vault in pure Go, so it is
  `assertable-in: {any}`.
- **"Gate wired"** is `class: liveness`, but it is **two checks, not one with a
  footnote**: `gate-committed-config` reads the *committed* `.claude/settings.json`
  (`assertable-in: {session, ci}` — CI sees the repo file) and `gate-effective-local`
  reads the *merged* config including a machine-local `settings.local.json` override
  (`assertable-in: {session}` — CI cannot see, and does not own, the dev machine's
  local layer). A single "gate wired" node carrying both assertabilities cannot be
  represented in this model, and the attempt to fake it (a `ci*`-with-footnote mask)
  is the special-case the two-axis split exists to abolish. So it must be split.

**`assertable-in` is *derived from what a check reads*, not hand-assigned** — this is
what makes the model mechanical rather than a growing exception list. A check over
**committed repo state** (a tracked file, the git config in the clone) is
`ci`-assertable; a check over **machine/effective state** (PATH, merged local config,
installed binary version) is `session`-only; a **hermetic** check (the live-fire temp
vault) is `any`. Read these off the data source; if a single node seems to need two
masks, it is reading two sources and must be split (the `gate-*` pair above is the
worked example).

The exit code in a given context is computed over checks whose `assertable-in`
includes that context; everything else is reported but exit-neutral there.

## The check DAG

doctor is a DAG of checks: nodes are checks, edges are **preconditions**. The engine
topologically orders the DAG, runs every node whose preconditions all passed, and
emits `skip(reason, blocked-by: <upstream>)` for any node gated by a failed
precondition. **No short-circuit** — a failing check never suppresses unrelated
branches; it only skips its own descendants.

Indicative structure (precondition → dependent), naming **nodes** (the positive
property — see the naming rule below):

- `vault-discoverable` → `config-valid`, `manifest-present`, `ignore-correct`,
  `grant-fresh`, `tool-read-files-present`
- `manifest-present` → `manifest-schema-readable` → `manifest-fresh`
- `binary-on-path` → `binary-schema-compatible`, `live-fire`
- `git-repo` → `precommit-anchor-live`, `grant-fresh`
- `agent-family-detected` → `gate-committed-config`, `gate-effective-local`,
  `postwrite-hook-live`, `no-legacy-broad-deny`

A precondition failure is *why* the dependent skips, and the skip line names it —
so the degenerate cases below produce a coherent report, not a cascade of
look-alike failures.

### Naming: nodes name the property, tokens name the failure

Two spellings are unavoidable and both are needed; the rule is to keep them in their
lanes and map between them once:

- A **node** is named for the **property it asserts**, phrased positively
  (`binary-on-path`, `manifest-present`, `gate-committed-config`, `live-fire`,
  `grant-fresh`). Nodes are what the DAG and preconditions reference.
- A **token** is the **stable wire value of a specific failure** a node emits,
  phrased as the finding (`binary-not-on-path`, `manifest-not-found`,
  `live-fire-not-denied`). Tokens are what a downstream consumer branches on, and
  they are the contract the summary and the ADR-0031 cross-ref lean on.

One node emits **one-to-many** tokens (the gate node alone emits `gate-missing`,
`gate-unresolved`, `gate-matcher-partial`). The catalog below is the **canonical
node→token map**; nothing else may introduce a token spelling. A consumer told to
"branch on token X" reads the token column, never a node name.

### Per-check catalog (canonical node→token map)

Each row is one node with its emitted tokens, `(class, assertable-in, severity)`, and
a remediation. The v1 verb's prose `reason` strings are retrofitted to these tokens.
(`session, ci` etc. is read off the data source per the rule above; nothing is
asterisked.)

| node | emitted token(s) | class | assertable-in | severity | remediation |
|---|---|---|---|---|---|
| `gate-committed-config` | `gate-missing`, `gate-unresolved`, `gate-matcher-partial` | liveness | session, ci | error / error / warning | `memento init` |
| `gate-effective-local` | `gate-locally-overridden` | liveness | session | error | restore local config |
| `postwrite-hook-live` | `postwrite-hook-missing` | liveness | session, ci | warning | `memento init` |
| `no-legacy-broad-deny` | `legacy-broad-deny-wired` | liveness | session, ci | error | remove legacy guard |
| `precommit-anchor-live` | `precommit-shadowed` | liveness | session, ci | error | unset `core.hooksPath` / compose memento step |
| `binary-on-path` | `binary-not-on-path` | liveness | session | error | install memento |
| `binary-schema-compatible` | `binary-schema-too-old` | liveness | session | error | upgrade memento |
| `live-fire` | `live-fire-not-denied` | liveness | any | error | upgrade / reinstall memento |
| `manifest-schema-readable` | `manifest-schema-unreadable` | hygiene | any | error | upgrade memento |
| `manifest-present` | `manifest-not-found` | hygiene | session, ci | warning | `memento compile` |
| `manifest-fresh` | `manifest-stale` | hygiene | session, ci | warning | `memento compile` |
| `config-valid` | `config-invalid` | hygiene | any | error | fix `.memento/config.toml` |
| `vault-discoverable` | `vault-ambiguous`, `vault-absent` | hygiene | any | error | set `MEMENTO_VAULT_ROOT` |
| `ignore-correct` | `gitignore-stanza-missing` | hygiene | any | warning | `memento init` |
| `tool-read-files-present` | `writing-md-absent` | hygiene | session | nudge | author a writing convention |
| `byo-conformer-resolvable` | `byo-conformer-unresolved` | hygiene | session | warning | fix conformer path |
| `grant-fresh` | `grant-stale` | liveness | session | warning | commit or drop the grant |

## Severity and exit contract

Four outcome levels plus skip:

- **`error`** — the vault is unusable or enforcement is off (e.g. `manifest-not-found`,
  `binary-schema-too-old`, `live-fire-not-denied`, `precommit-shadowed`). Flips the
  headline to a hard failure.
- **`warning`** — degraded but usable (e.g. a dead PostToolUse hook, a stale manifest).
- **`nudge`** — advisory (e.g. ADR-0010 "no `writing.md`").
- **`ok`** — passed.
- **`skip`** — precondition failed; exit-neutral but rendered as not-green with its
  blocking reason. (The v1 verb dishonestly reports skipped checks as `ok` — e.g.
  `staleGrantCheck` returns `statusOK "not checked"` with no vault; this fixes that.)

**Exit codes keep the established spine** — `2` is already "invalid arguments"
CLI-wide (`parseSubcommandFlags`), so it is *not* repurposed for a severity tier:

- `0` — no gating finding in this context.
- `1` — at least one gating finding (an `error`, or a `warning` under strict).
- `2` — usage error (unchanged).

**Does a warning exit nonzero?** Not by default — human-glance stays quiet.
`MEMENTO_DOCTOR_STRICT` (truthy, parsed exactly like `MEMENTO_STRICT_COMMIT`)
**promotes `warning` to a gating finding**, so CI opts in. This deliberately reuses
the detection-default / mitigation-opt-in idiom `compile` already established rather
than inventing a second policy surface. `nudge` never gates.

The exit code is computed over `{checks whose assertable-in includes the current
context}` — so a CI run is never failed by a `session`-only liveness property it
cannot assert, which is the [[doctor-scoping]] cadence constraint expressed
mechanically rather than by splitting verbs.

## Liveness ≠ presence

A `stat` + `grep` proves a file exists with the right substring. It does **not**
prove the hook *fires*. The pre-commit anchor is the sharp case, and it is wholly
unchecked today (the v1 verb checks PreToolUse/PostToolUse/binary/live-fire/grants —
**not** the git pre-commit hook, which is the only liveness anchor for the
ratification-boundary MODE VIOLATION audit, the integrity floor of ADR-0031).

The liveness predicate that matters is **behavioral**: *does a commit touching a note
cause the manifest to recompile.* doctor is diagnose-only, so it cannot run a commit
to find out; it approximates with the **strong, behavioral-proxy check** —

- **Shadowing test (strong, `error`):** resolve the hook path git will *actually*
  use, honoring `core.hooksPath` and third-party managers (husky, lefthook,
  pre-commit-framework), and verify memento's invocation is reachable within the
  effective pre-commit hook. A `core.hooksPath` redirect that bypasses
  `.git/hooks/pre-commit` makes a byte-perfect installed script **dead** — and
  nothing detects this today.

and demotes script identity to a **weak signal at most**:

- **Content-identity drift (weak, `nudge` — never `error`):** doctor must *not*
  brittle-match the installed script byte-for-byte against a baked-in expected
  string. Legitimate composition (someone folds memento's step into a larger
  pre-commit) would then report as drift. Content-matching fails open (script edited
  but still works → false alarm) *and* fails closed (script byte-identical but
  `core.hooksPath` redirects past it → looks fine, is dead). So the strong check is
  shadowing/reachability; identity is a nudge, never a gate. This avoids reinventing
  the stale-hook detector as a brittleness generator.

## Manifest freshness

The freshness check is an **authoritative, side-effect-free in-buffer recompile +
diff**: doctor recomputes the manifest in memory and compares it to the on-disk
artifact, reporting `manifest-stale` on mismatch. It **never writes** — writing
would race the PostToolUse compile hook, and a diagnostic must not mutate the thing
it diagnoses.

This is the same predicate as the `brief`/`read` freshness check (memento-7z4), at
**higher fidelity and higher cost**:

- **doctor** is a cold path that runs occasionally, so it affords the authoritative
  content recompile+diff.
- **brief/read** is a hot path, so memento-7z4 uses a cheap **mtime heuristic**
  (max note mtime vs manifest mtime).

The **permitted error has a direction, and it is deliberate**: the mtime heuristic
can report *fresh* when a content diff would report *stale* — a touch-without-change,
or clock skew across clones. So brief's cheap check is **allowed to miss staleness
that doctor catches**. This is an accepted soundness-for-cost trade, *not a bug*:
stating the direction explicitly is load-bearing, because a future reader who
"fixes" brief to match doctor reintroduces the per-call recompile cost on the hottest
verb that memento-7z4 exists to avoid.

### The CI gate rests on a compile-determinism invariant — named, not assumed

`manifest-stale` is `{session, ci}` and promotable to a hard gate under
`MEMENTO_DOCTOR_STRICT`. A recompile-and-diff is only sound as a **CI** gate if
`compile` is **byte-stable across OS/arch** — the committed manifest is produced on
one developer's platform (e.g. macOS arm64) and the gate recompiles on another (e.g.
the CI runner's Linux amd64). If the manifest embedded anything path-separator-,
locale-ordering-, or wall-clock-sensitive, the runner's recompile would diverge from
the committed bytes and `manifest-stale` would fire on a **phantom**, gating green
builds by construction. So:

- **Named precondition.** Freshness assumes `compile` is deterministic across
  platforms: keys are slash-normalised (ADR-0020 vault-walk portability), entries are
  sorted by key, content fields are hashes, and `updated` is frontmatter-derived, not
  file mtime. This invariant is a **hard dependency of the freshness check**, recorded
  here so it is maintained deliberately rather than discovered when CI flakes.
- **Diff the canonical projection, not raw bytes.** The comparison runs over the
  decoded, canonicalised manifest model (the same projection both sides compute), so a
  re-serialisation, key-ordering, or whitespace difference can never raise
  `manifest-stale` — only a genuine corpus-vs-manifest divergence can.
- **Fallback if the invariant ever fails to hold.** If a future manifest field is not
  platform-stable and cannot be canonicalised away, `manifest-fresh` must drop to
  `assertable-in: {session}` — keep it as a local nudge rather than ship a
  flaky-by-construction CI gate. The gate meaning something is worth more than the
  gate existing.

## Schema-compat: one fact, two faculties

"Compare manifest `schema_version` to the binary's `CurrentSchemaVersion`" is one
comparison serving two faculty-regions, currently fused inside `binaryReachableCheck`
(`internal/cli/doctor.go:326`). The ADR **splits it into two nodes** that may share
the underlying comparison but are reported as distinct findings with distinct class
and assertability, because they answer different questions:

- **`binary-schema-compatible` (`class: liveness`, `assertable-in: {session}`)** — the
  binary the *gate shells to here* cannot read the vault it guards, so it enforces
  nothing. A per-machine enforcement-is-off property (emits `binary-schema-too-old`);
  CI's binary version says nothing about the dev machine's, hence `session`-only.
- **`manifest-schema-readable` (`class: hygiene`, `assertable-in: {any}`)** —
  independent of any hook: can a memento binary at this schema decode the committed
  on-disk manifest at all. A static fact about the artifact, true even where no gate
  is wired, and assertable anywhere a clone exists (emits `manifest-schema-unreadable`).

Distinct, too, from `compile`'s **write-time** validity (can the binary *produce* a
manifest at the current schema). Three different questions that the v1 fusion blurred.
The two doctor nodes are *not* the same predicate twice: one is keyed on the gate's
resolved binary (`${MEMENTO_BIN:-memento}`), the other on the artifact-vs-schema fact,
and they diverge whenever the gate's binary is not the one running doctor.

## init ↔ doctor symmetry (one-directional)

**Every invariant doctor checks should be something `init` establishes** — so a
future `--fix` re-invokes init sub-steps rather than growing a parallel
implementation, and the DAG's per-node remediation is "re-run init step N." This is
promoted from a proposal to a **load-bearing invariant** of the design: it is a
forcing function on scope (if doctor wants to check X, init must establish X, else X
is not doctor's business — this is exactly what keeps the `writing.md` *presence*
nudge in and its *quality* out).

But it is **one-directional**: *every init invariant is doctor-checkable* is the
invariant; *every doctor check is an init invariant* is **false**. The residue that
breaks the converse is the **environment / global-git class** — `binary-on-path`,
`core.hooksPath` redirection — which init does not (and cannot) establish. For that
class `--fix` can only **advise** (print the install URL, the `git config` command),
never re-run an init step. The ADR names this exception class explicitly so the
`--fix` design does not inherit a silent hole.

## Degenerate cases (the most common invocations)

doctor is most often run with **no vault**, **outside a project**, or in a **non-git
tree**. Each must produce a graceful report with a specific exit code and **never
panic**:

- **No vault / run outside a project** — `vault-discoverable` fails as the DAG root;
  every vault-dependent check `skip`s with `blocked-by: vault-discoverable`. doctor
  reports "no memento vault here" and exits with a specific code, not a cascade of
  look-alike errors and not a crash.
- **Ambiguous vault** (multiple markers) — reported as `vault-ambiguous` (`error`),
  remediation `MEMENTO_VAULT_ROOT`, consistent with the `vault_discovery_ambiguous`
  reason_code ADR-0031 defines for the gate.
- **No git** — *supported*, with a named capability loss: ratification is
  undeterminable, so **grant-staleness cannot be evaluated** (it needs `git status`)
  and `precommit-anchor-live` is moot (no `.git/hooks`). Both `skip` with
  `blocked-by: git-repo`. The live-fire probe is git-independent (it builds a
  non-git temp vault), so enforcement liveness is still assertable without git.

## v1 boundary and deferred

- **Diagnose-only.** v1 prints findings and remediation commands; it runs nothing
  and writes nothing — including freshness, which compiles to a buffer and never
  touches disk.
- **`--fix` deferred.** When it lands it re-invokes init sub-steps per the symmetry
  invariant; orphan cleanup (retired write-skill, legacy hook entries — [[doctor-scoping]])
  is its first job, with the standalone migration bead as the interim owner until then.
- **Token retrofit.** The v1 verb's prose `reason` strings become stable tokens; the
  headline keeps the loud `vault write enforcement: LIVE / OFF` line as the
  liveness-class summary.

## Open questions / deferred

- **`assertable-in` defaults and override.** Whether the context is auto-detected
  (e.g. `CI` env var → `ci`) or always explicit, and whether a flag can widen the
  mask, is left to implementation.
- **Per-note "form" checks** ([[review-audit-doctor faculties]]) — frontmatter/style
  well-formedness is mechanical but judges a note, so the discriminator places it in
  `review`/`compile`, not doctor. Reconfirmed here, not reopened.
- **Programmatic consumption / `audit` faculty** — a machine-readable doctor output
  (JSON) is plausible but unscoped; it rhymes with the deferred `audit` faculty.

## Related

- [[doctor-scoping]] — the candidate-check inbox and three-cadence analysis this ADR
  converges and commits.
- [[adr-0031-remove-write-verb-hook-enforced-native-writes]] — names doctor a hard
  dependency and the only loud liveness surface; this ADR is the deferred follow-up,
  and the shipped liveness verb is its `liveness`-class subset.
- [[adr-0029-convention-files-and-convention-verb]] — its convention-check routing is
  split (structure→compile, taste→review) by the discriminator.
- [[adr-0010-tool-read-writing-guide]] — its deferred "missing `writing.md`" signal
  lands here as a `nudge`.
- [[review-audit-doctor faculties]] — the review/audit/doctor faculty carve this
  discriminator refines.
- memento-7z4 — the `brief`/`read` mtime-freshness bead; shares doctor's freshness
  predicate at lower fidelity, with the permitted-error direction stated above.
- memento-0j8 — the tracking issue for this ADR.
