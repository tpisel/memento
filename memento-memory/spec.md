---
title: memento — design specification
status: draft
version: 0.2
updated: 2026-06-13
summary: Post-ADR alignment spec for memento's vault model, manifest and brief artifacts, `_memento/` namespace, bootloader flow, and v0-v4 scope boundaries.
---

# memento

A thin retrieval/dispatch layer over a human-curated markdown knowledge store. It compiles a manifest that agents load at session start, and exposes selective read/write so an agent pulls *decomposed* context on demand rather than ingesting the whole store every time.

The store is managed by a human first (Obsidian as the authoring/browsing surface) and consumed by an agent second. There is one source of truth — the markdown files on disk — and everything else (the manifest, the link graph) is derived from them, so the human's view and the agent's view cannot drift.

---

## 1. Problem & position

Coding agents have no persistent context across sessions. The standard responses each fail in a characteristic way: markdown TODO/plan files are write-only memory that fragments over long sessions; dumping everything into `AGENTS.md`/`CLAUDE.md` blows the context budget and trains the agent to skim; key-value memory stores (e.g. beads' `bd remember` → `bd prime`) inject *all* memories verbatim at prime time, which is a naive full dump dressed as retrieval and only stays cheap through manual curation discipline.

memento targets the **durable semantic layer** specifically, and leaves the other layers to the tools that already do them well. The working model is three substrates distinguished by *access pattern* — the only axis an agent actually experiences at runtime:

| Substrate | Content | Access pattern | Owner |
|---|---|---|---|
| `AGENTS.md` / `CLAUDE.md` | Invariants, hard rules, bootloader | Unconditional push, every session | hand-authored |
| beads | Task state, working memory | Scheduled pull at checkpoints (`bd ready`) | agent + human |
| **memento** (resolved vault; `<project>-memory/` by default) | Decisions, discoveries, learnings, specs, ADRs | Conditional, agent-initiated pull, front-loaded to task-start | human-first, agent-augmented |

Note that "always-load slice" is **not** a third kind of content — it is a *loading policy* (a pin) applied to a slice of the durable layer, and in practice it lives in `AGENTS.md` as the bootloader. The bootloader's single most important job is to teach the agent how to use the other two substrates: *current work lives in beads (`bd ready`); durable knowledge lives in the resolved memento vault, here is the brief and how to query it.* If `AGENTS.md` does nothing else well it must do this — it is the load-bearing member, and the only thing that degrades everything downstream at once if it bloats or goes stale.

### Code is prime memory

The code is ground truth for *what is*. The durable layer should therefore hold only what is **not recoverable by reading the code**: the *why*, the rejected alternatives, the discovered-the-hard-way constraints, the tribal knowledge with no representation in the AST. A "learning" that restates what the code says is redundant and should be deleted. This is what keeps the semantic layer a high-signal residue rather than a second prose copy of the repo, and it is what makes the per-task retrieval cost worth paying.

---

## 2. Architecture

**Library core, thin transports.** All work — walk vault, compile manifest, read a doc, write a doc with mode validation — lives in a package with no knowledge of how it is invoked. The CLI is a shell over that package. The MCP server (later) is a *second* shell over the identical package, registering the same functions as tools. Get this boundary right and the MCP is a `serve` subcommand, not a rewrite.

Core API (illustrative):

```go
type Manifest struct { Entries []Entry; Tags map[string]int }
type Entry struct {
    Key, Title, Summary, Path string
    Tags     []string
    Headings []string            // depth-capped (H2/H3)
    Mode     WriteMode
    Updated  time.Time
    Links    Links               // out + in (compile-time product)
    SummaryStale bool
}

Compile(vault string, cfg Config) (Manifest, error)   // pure, deterministic, no network
Read(vault, key string) (Doc, error)                  // supports key#heading
Write(vault, key, content string, mode WriteMode) error // validates op against declared mode
List(vault string, f Filter) ([]Entry, error)
```

CLI subcommands and MCP tool registrations both call exactly these. Neither contains business logic.

**Language: Go.** Single static binary (matches the beads distribution story — curl-install, no runtime), good MCP stdio-server story, sub-second vault walks. Homebrew name `memento` is clear.

**The `read`/`write` CLI verbs are built early even though a human rarely types them**, because they are the exact surface the MCP will expose. By the time `serve` is written, that API has been exercised through CLI use and ossified into something trusted — the MCP tool surface is *promoted from* a battle-tested CLI API, never designed speculatively.

---

## 3. The memory directory & vault model

- **Default directory: `<project>-memory/`**, parametrised (`--dir`, config), per ADR-0001. `<project>` is derived at `init` time from the git remote basename, falling back to the repository directory name; if no project name can be derived, fall back to `memory/`. The default remains deliberately **not** `docs/`, which already means published documentation, API-doc output, and doc-site sources (mkdocs/docusaurus/sphinx); landing agent memory there tangles it with human-published docs and gets caught by site builders. A dotfolder (`.agent/`) would avoid collision but Obsidian hides dot-prefixed folders, breaking the human-first browse.
- **Discovery is marker-based**, per ADR-0002. Commands discover the repository vault by finding exactly one `<memory-dir>/.memento/` marker directory; ambiguity is a hard error. Discovery does not depend on the vault directory name, so a vault can be renamed as long as its marker moves with it.
- **The human opens the resolved memory directory itself as the Obsidian vault root**, not the repo root. This bounds the wikilink graph to the memory store; opening the repo root would let links leak into source files and pollute the out/in-link surface. (Human-setup note for the docs; the tool only walks what it is pointed at.)
- The store is **in-repo** so it version-locks to the code and travels with branches.

---

## 4. The manifest

A compiled, **committed** artifact — the canonical machine index and review surface. Committing it makes changes to project semantic memory show up in review (the beads-in-git property applied to the durable layer) and means the agent-facing brief can be derived from a static file with zero build step. Regenerated via a pre-commit hook (sub-second, so invisible); stale-committed-manifest is the obvious risk and the hook closes it, while a diffable manifest lets a human catch rot in PR.

Canonical machine form is **JSON** (deterministic to parse), emitted at `<vault>/.memento/manifest.json` per ADR-0002. Per ADR-0008, `compile` also emits `<vault>/_memento/brief.md`: a derived, gitignored, markdown projection of the manifest for agent context. The bootloader injects `memento brief` output rather than raw JSON. The brief is not a second source of truth; it is regenerated from the canonical manifest.

### Per-file fields

- **key** — vault-relative path (see §5).
- **title** — frontmatter `title:`, else H1, else filename.
- **summary** — see §8 fallback chain.
- **tags** — from frontmatter.
- **headings** — the H2/H3 tree (depth-capped to stay scannable). This is most of the value of reading the doc at a fraction of the tokens, and is what enables section-level reads (§7).
- **mode** — write-mode (§9).
- **updated** — timestamp.
- **summary_stale** — detection flag (§8).
- **links** — out + in (computed at compile time; consumed at v3, §7).
  - outlinks are objects with `target`, `type`, and `resolved`; resolved targets use the manifest key, unresolved targets keep the raw wikilink target.
  - inlinks are inverted resolved edges with `source` and `type`.

### Global

- **tag vocabulary with counts** — lets the task-start scan filter by domain rather than read everything; also a rot signal (count-1 tags are usually typos; a sprawling vocabulary means tagging discipline slipped).

Compile is a **full stateless rebuild** each run. Entries are emitted from what exists; deletions and renames simply produce a manifest without the old entry — no diffing or patching state to maintain.

---

## 5. Addressing & renames

**key = vault-relative path; the human view is canonical.** Renames propagate into the next manifest (old entry vanishes, new one appears). This resolves the "human says *look at file X* while the agent now sees ref Y" hazard — what the human sees in Obsidian *is* the address.

- Intra-vault `[[wikilinks]]` are auto-rewritten by Obsidian on rename, so internal references stay valid for free.
- The only exposure is references that have **escaped the vault** — a beads issue, a commit message, a code comment, an ADR cited by name elsewhere. Those break silently on rename.
- A frontmatter stable-`id:` system was considered and **rejected** as added authoring friction that diverges from how wikilinks actually address. Where permanence genuinely matters (ADRs), it is enforced by **social convention** — *never rename an accepted ADR; its number is its identity* — not by machinery.

---

## 6. The ignore taxonomy & operational files

Three states, not two:

1. **Content** — indexed, readable. The normal case.
2. **Human-only** — ignored entirely (daily notes, scratch, meeting jots). Listed in the ignore file.
3. **Operational** — tool-relevant material read by the tool at specific moments. Machine-owned operational files live in `.memento/`; human-readable tool artifacts and vault conventions live in `_memento/` per ADR-0009.

**`.mementoignore`** — a dotfile at the memory root, hidden from Obsidian and out of the content namespace, using a small gitignore-like syntax with `#` comments. The comments make it self-documenting (a soft demo to the user of what the file is for) while keeping parsing unambiguous — a markdown ignore would force the parser to disentangle prose from globs. The ignore file ignores itself.

**Operational-ness is a named role, orthogonal to ignore membership.** `.memento/manifest.json` is machine-owned and hidden. `_memento/brief.md` is generated and ignored. Future user-authored files such as `_memento/writing.md`, `_memento/review.md`, and `_memento/audit.md` are operational because tools look for those named roles, not because the files appear in an ignore list. Do not derive operational-ness from ignore membership — the ignore file lists many things that are merely human-only.

**Layout: two tool namespaces.** `.memento/` is the hidden machine namespace for `config.toml`, `manifest.json`, and future structured tool state. `_memento/` is the Obsidian-visible, mixed-audience namespace for tool-relevant human-readable artifacts: generated files such as `brief.md` and versioned user-authored guides or prompts. Ownership inside `_memento/` is file-level, not folder-level; generated files are ignored individually, while user-authored convention files remain versioned by default. Specific tool-read filenames are deferred to ADR-0010.

---

## 7. Reading

- **Whole-file read:** `read <key>` returns the body.
- **Section read:** `read <key>#<heading>` returns a single section, anchored on the heading tree already in the manifest. This is **decomposition at read-time** — the agent sees a doc's H2 outline in the manifest and pulls only the relevant section — and it is the answer to "how do I keep big specs usable without splitting them into files." Pulled forward to **v1/v2**, not deferred.
- **Links (v3 consumption):** read surfaces a doc's **outlinks and inlinks** so the agent can navigate to more. Inlinks require the whole-vault graph (you cannot know what points *to* X by reading X), but this is computed at **compile time** and stored in the manifest, so read simply surfaces what is already there — the manifest is a runtime input to read, not just a session-start load.
- **Transclusions (`![[x]]`) are NOT resolved or inlined.** Auto-inlining means a doc that embeds five others pulls all five on read — the load-everything problem in a costume, directly against the decomposition goal. Transclusions are surfaced as an **`embed`-typed outlink** instead; the agent chooses whether to pull the target.
- **Typed links** (`depends-on`, `see-also`, `supersedes`, `embeds`) let the agent traverse selectively rather than chase every organic human association (those are great for serendipitous human browsing, noisy as agent traversal edges). The typed-edge overlay is **grown from observed transitive-relevance misses**, not built speculatively — flat tag-filtered retrieval is the spine; edges are added where a real dependency would otherwise be missed.

**Retrieval instruction (lives in the bootloader):** *before starting a task, scan the manifest's titles + summaries + headings, filter by tag to the task's domain, and read the bodies (or sections) of entries that plausibly apply.* Front-loading the retrieval decision to task-start — when the agent has the clearest model of what the task touches — beats hoping it realises mid-implementation that it needed context. This converts unknown-unknowns into known-unknowns: the agent cannot search for a constraint it does not know exists, but it can recognise relevance in a scannable list of summaries.

---

## 8. Writing

### Modes (frontmatter-declared, tool-enforced)

| Mode | Semantics | Typical use |
|---|---|---|
| `append-only` | New content appended; nothing overwritten | Decision logs, ADR history |
| `section-replace` | Overwrite a named heading's section; others untouched | Living reference docs |
| `keyed-upsert` | Add or update structured entries by key | Discoveries, constraints |
| `read-only` | Readable, not writable via the tool | Frozen specs, accepted ADRs |

The tool **validates the operation against the declared mode before writing**. This is the thing the MCP gate makes reliable that prompt-instruction cannot: an agent can rationalise past a written rule, but a `read-only` doc is *physically* unwritable through the write tool — to change an accepted decision the agent **must** author a superseding record, not quietly rewrite history.

### When the agent should write of its own accord (default triggers)

Trigger-shaped, not discretion-shaped — discretionary "write down useful things" yields noise-or-nothing; specific triggers yield signal. These live in a tool-read writing guide so they are tunable per-project without a recompile, and are **read at write-time**. ADR-0010 pins the concrete `_memento/` filename and precedence rules.

**Write when:**
- you discovered a constraint not evident from the code and not already in the memento vault;
- a decision was made (or handed to you) with non-obvious rationale or rejected alternatives → new ADR;
- you corrected an assumption already recorded in the memento vault that is now wrong.

**Do not write** (the negatives prevent the swamp, and matter as much as the positives):
- anything that merely restates what the code says;
- task state — that belongs in beads;
- transient/session-specific detail;
- anything you are guessing at.

### Posture

**Autonomous write with *asynchronous* review via git diff.** Do not gate writes behind synchronous human approval — that kills the agent's flow. Rely on the committed-manifest-is-diffable loop: agent writes land as diffs a human sees in PR, which is where rot is caught. The boundary leak to watch: agents will try to encode durable learnings into beads close-notes, where compaction destroys them — the bootloader and writing guide must state that discoveries outliving a task exit beads into the memento vault.

---

## 9. Summarisation

**Compile is pure.** Deterministic, no network, hook-safe — it must never make an LLM call, because it runs on a pre-commit hook and a hook that hangs on a flaky/slow/costly network call is unacceptable. Compile therefore does **detection only**: it flags which files are new, summary-less, or body-changed-since-summary.

**Trigger = body-content hash, not mtime.** mtime is destroyed by `git checkout`/`clone` and would misfire constantly in any multi-machine or CI setting. The tool stores `summary_hash` (sha256 of the body, **excluding frontmatter**) in frontmatter; a file is stale when its current body hash ≠ stored hash. This also kills the regeneration loop for free: writing a summary mutates frontmatter but not body, so the body hash is unchanged and no re-trigger fires.

**Generation is a separate, explicitly-invoked step**, and *who* generates depends on who is driving:

- **Agent-driven (MCP, v3):** the tool returns *"these N files need summaries, here are their bodies"*; the agent writes summaries back through the write tool. This borrows the calling agent's compute — **no API key lives in the tool**. This is the primary path and it is unlocked *by* the MCP, not a later add-on.
- **Standalone CLI (v4, optional):** a `--summarize` flag shells out to a configured model (e.g. the `claude` binary). Only matters when no agent is in the loop.

So "auto-summarisation" is not a single feature at a fixed version: *detection* ships in v0; *agent-generation* arrives with the MCP at v3; *standalone CLI generation* is a v4 optional. Capability tracks who is driving.

---

## 10. CLI surface

```
memento compile          # walk vault → emit manifest (or stdout with --print for testing)
memento brief            # print the agent-facing manifest projection
memento init             # adopt-or-create: scaffold/adopt the vault, hook, bootloader (§11)
memento read  <key>      # whole-file; supports read <key>#<heading>
memento write <key>      # append/upsert/section-replace, validated against declared mode
memento serve            # MCP server (v3) — registers compile/read/write/list as tools
```

`compile` flags: `--dir`, `--print` (stdout, no file), `--summarize` (v4).

---

## 11. `init` & adoption flows

**Adoption is the primary path; greenfield scaffolding is the minority case.** Most real projects already have scattered markdown, an existing `AGENTS.md`, maybe an ADR directory, maybe a vault. Designing `init` as create-only would be backwards.

`init` is **adopt-or-create**:

- Pointed at a **non-empty** dir → **adopt**: compile what is there, inject the bootloader, drop operational files (`.memento/` marker/config, `.mementoignore`) only if absent. Never clobber.
- Pointed at an **empty/new** dir → create the minimal structure plus a single example note with model frontmatter (convention-by-example).

**Compile must work on bare, frontmatter-less markdown** (filename → title, first line → summary, flag the gap). Frontmatter is **progressive enhancement** the human adds incrementally, never a precondition — a hard frontmatter requirement would turn adoption into a retrofit wall and kill it on contact with any existing repo.

**Bootloader injection** is a **sentinel-bounded block** in `AGENTS.md`/`CLAUDE.md`:

```
<!-- memento:start -->
Durable project knowledge lives in `<vault>`.
Run `memento brief` to load the agent-facing manifest projection (titles, summaries, tags, headings, modes).
Identify relevant entries from the brief; read only the bodies or sections that plausibly apply with `memento read <key>`.
Working state lives in beads (`bd ready`); discoveries that outlive a task go to `<vault>/`, not beads notes.
<!-- memento:end -->
```

Idempotent and removable (re-running replaces the block; never blind-appends). The block is **parametrised by the resolved dir** and points agents at `memento brief`; the canonical manifest remains at `<vault>/.memento/manifest.json`. Critical, catastrophic-if-wrong items are promoted *out* of the vault and into `AGENTS.md` directly — the head of the distribution is unconditional, the long tail is conditional.

**Obsidian config is not owned.** `init` does not create or manage `.obsidian/` — a vault is just a folder and Obsidian creates its own config on first open. The only Obsidian-aware action is a `.gitignore` stanza for the per-machine UI noise (`.obsidian/workspace*`, `.obsidian/cache`), which is git hygiene, not config ownership.

`init` runs once per project — **do not gold-plate it.** The durable engineering value is in `compile` and the core read/write API.

---

## 12. Spec-driven development, ADRs, and what we deliberately do not build

- **Spec-driven dev works natively** — a spec is just a durable doc with frontmatter, in the manifest like anything else. The heading-tree + section-read combination makes a monolithic spec navigable without splitting it.
- **Auto-decomposition of specs into subfiles is NOT built.** Splitting along seams is a semantic authoring act; baking it into the index layer conflates indexing with content-transformation. If wanted, it is an *agent task* ("split this along its H2s into linked notes"), and memento *supports* the decomposed result (heading tree + outlinks) without *performing* the split.
- **ADRs are the paradigm case that justifies the write-mode system.** An accepted ADR is `mode: read-only`, carries a `status:`, and supersession is a *new* ADR with a `supersedes:` typed link back. Ship a default ADR frontmatter convention in the tool-read writing guide once ADR-0010 pins the filename.
- **Obsidian Tasks: no integration.** Tasks-in-markdown is a competing task store in the durable substrate — exactly the working-memory-into-semantic-memory leak beads exists to prevent. Human task-jotting in notes falls on the human-only ignore side; the agent must not treat it as authoritative state.

---

## 13. Version / scope plan

| Ver | Scope |
|---|---|
| **v0** | CLI `compile`, `init`, `read`, and minimum `write`. Point at or discover one marker-based vault → canonical `.memento/manifest.json` plus generated `_memento/brief.md` (`--print` to stdout for testing the JSON representation). Includes: `<project>-memory/` init default, `.mementoignore` (subdir walking, glob+comment syntax), tag vocabulary (omitted if no tags exist), heading extraction, out/in link graph as data, bare-markdown fallback, summary-staleness **detection** (flag only, no generation), adopt-or-create init, pre-commit hook, sentinel bootloader injection, `.gitignore` stanza, `memento brief`, whole-file and `#heading` reads, and conservative write support limited to create/append. |
| **v1** | Hardening and polish around the v0 surfaces after dogfooding: better diagnostics, portability fixes, and any compatibility adjustments needed before the MCP surface is promoted from the CLI. |
| **v2** | Smarter writes. Tool-read conventions such as `_memento/writing.md` expose agent-facing write rules / triggers / placement conventions once ADR-0010 pins filenames and precedence. Full mode-aware editing, including `section-replace`, `keyed-upsert`, and mechanical `read-only` enforcement. Default ADR convention. |
| **v3** | MCP server (`serve`). `read` surfaces out/inlinks for navigation. Agent-driven summarisation (borrow caller compute). |
| **v4** | Standalone CLI auto-summarisation (`--summarize`, configured model). `review` verb for deterministic and agent-assisted maintenance. CLI verb to open Obsidian pointed at the resolved vault. |

---

## 14. Design principles (non-functional)

- **Compile is pure, stateless, sub-second, hook-safe.** No network, full rebuild each run.
- **Human-canonical.** What the human sees in Obsidian is truth; the agent's address space is the human's path space.
- **Single source of truth.** The markdown files; the manifest and link graph are derived and cannot drift from them.
- **Diffable = auditable.** Committed manifest + git makes semantic-memory changes and rot visible in review — the cheapest defence against silent decay.
- **Structure earns itself from observed failure, not a-priori tidiness.** Applies to the typed-link graph, the `_memento/` filename set, the embedding index, and the taxonomy itself — add cardinality when a failure mode demands it, not before.
- **Transports are dumb; the core is shared.** The MCP is a second mouth on the same library, promoted from a dogfooded CLI API.

---

## 15. Deferred / out of scope / open questions

- **Typed-link traversal policy** — which edge types the agent follows by default. Defer; grow from misses.
- **Embedding index over summaries** — only if the flat manifest visibly strains (low hundreds of docs). The first response to manifest bloat is *pruning the store*, not adding a graph. Defer.
- **Monorepo / multiple memory dirs / manifest-of-manifests** — out of scope; the "point at a folder" design does not preclude it.
- **Standalone summariser model wiring** — v4 detail (which binary, config surface, prompt).
- **Vault-boundary enforcement** — currently a human-setup convention (open the resolved memory directory as vault root); no tooling guard.
- **ADR-0010 filename conventions** — pin `_memento/writing.md`, `_memento/review.md`, `_memento/audit.md`, and any precedence rules against memento-shipped defaults.
- **Token-aware brief sizing** — bytes/lines are the v0 size proxy; tokenizer-backed sizing waits until brief size approaches context budgets.
- **`init --template=` starters** — opinionated vault structures and starter conventions remain opt-in.
- **Doc-type-specific brief rendering** — ADRs, specs, and notes render uniformly in v0; specialized rendering waits for observed need.
- **Unresolved-question convention** — choose a home and naming scheme for open design questions, RFD-style or similar, alongside ADRs.


## Dogfooding note

We'll dogfood docs in our repo using the same method, though it's likely that we'll want to write ADRs and other files before the tools to do so are formally available. If this occurs, try to write docs as if those tools existed.

memento dir is `memento-memory/`
ADRs in `memento-memory/Architecture decision record/`
If a non-blocking open question poses itself in development, append to `memento-memory/Open questions.md`, similarly for `memento-memory/Feature ideas.md`
