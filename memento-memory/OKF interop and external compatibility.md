---
title: OKF interop and external compatibility
status: proposal
mode: append-only
date: 2026-06-14
tags: [memento, proposal, open-question, interop, okf, obsidian, format, distribution]
summary: Working notes on aligning memento with Google Cloud's Open Knowledge Format (OKF) v0.1. Records what we can align without harming the Obsidian-as-human-interface commitment, what gap remains, whether to surface OKF support as an explicit mode, and where the OKF spec appears to be heading that we may already be on the path to.
---

# OKF interop and external compatibility

## What this is

Google Cloud published the Open Knowledge Format (OKF) v0.1 on 2026-06-13 (blog: `cloud.google.com/blog/products/data-analytics/how-the-open-knowledge-format-can-improve-data-sharing/`; spec: `github.com/GoogleCloudPlatform/knowledge-catalog/blob/main/okf/SPEC.md`). It formalises the "LLM wiki" pattern — a directory of markdown files with YAML frontmatter — into a vendor-neutral interchange format. memento is shaped close enough to OKF that the question of *whether and how to converge* is worth recording before the design ossifies further in a direction that makes alignment costly.

This note is exploratory, not a commitment. It exists so a future ADR has the framing pre-loaded and does not have to re-derive the analysis cold.

## OKF v0.1 in two paragraphs

A **bundle** is a directory of `.md` files with YAML frontmatter. Concept ID = file path with `.md` stripped. Only `type:` is required; `title`, `description`, `resource`, `tags`, `timestamp` are recommended. Two filenames are reserved: `index.md` (a directory listing for "progressive disclosure," which in practice is a manually-or-tool-generated TOC) and `log.md` (a date-grouped changelog). Links are standard markdown, and OKF normatively states links are *untyped* — the relationship is conveyed by surrounding prose, not by the link itself. Consumers MUST tolerate broken links, unknown types, unknown frontmatter keys, and missing optional files.

Versioning is via `okf_version: "0.1"` in the bundle-root `index.md` (the only place `index.md` is allowed frontmatter). Minor bumps add backward-compatibly; major bumps may rename required fields or reserved filenames. Consumers that don't understand a declared version SHOULD attempt best-effort consumption, not refuse.

## What we can align without harming Obsidian-as-human-interface

Most of OKF is already what memento does, or what memento can do without touching the human surface:

- **Substrate is identical.** Markdown + YAML frontmatter, version-controlled directory tree. Obsidian opens an OKF bundle as a vault unchanged.
- **File path is concept identity.** memento already commits to this (spec §5; ADR-0007 pins key stability). The only difference is the `.md` strip — purely an export-side mapping.
- **Frontmatter as additional keys.** OKF's permissive consumption model means memento's `mode`, `summary_hash`, `updated`, and the rest are valid OKF extension keys. Producers may add, consumers MUST preserve. This buys us full retention of write modes, summary detection, and any future structured metadata *without leaving conformance*.
- **Typed links in frontmatter, not body syntax.** OKF says links in the markdown body are untyped. memento's planned typed-link overlay (`depends-on`, `supersedes`, `embeds`) lives in the compiled manifest, computed from frontmatter conventions and link context. As long as we never encode link types in the markdown body (no `[[target|type:depends-on]]`, no link-attribute extensions), we stay clean. This is a constraint we should pin now — it costs nothing today and keeps the export path open.
- **Bare-markdown bodies.** Both formats are content-agnostic about body structure. memento's heading-tree extraction and section-anchored reads operate on standard markdown headings; nothing OKF says forbids this.
- **`# Schema`, `# Examples`, `# Citations` conventional headings.** OKF names these as conventions, not requirements. memento's heading extraction picks them up uniformly. Free.
- **Distribution.** OKF lists "git repository (recommended), tarball, zip, or subdirectory within a larger repository." memento's in-repo vault model is the "subdirectory within a larger repository" case.

The Obsidian commitment (spec §3: human opens the resolved memory directory as vault root; wikilinks bounded to the store) is not threatened by any of the above. OKF says nothing about how a human authors; it specifies the on-disk shape and what consumers must tolerate.

## Where the gap remains

Three meaningful places memento and OKF do not line up:

### 1. Bare-frontmatter tolerance

memento's adopt-or-create flow (spec §11; ADR-0005) explicitly accepts markdown files with no frontmatter at all — filename becomes title, first line becomes summary, the gap is flagged. OKF conformance requires *parseable* YAML frontmatter with a non-empty `type:` on every non-reserved `.md` file.

This is not a conflict for internal use — bare-markdown tolerance is a human ergonomics property that lets memento be useful on day zero. It only matters on *export*: a memento→OKF export shim must synthesise `type:` for any file missing it. The reasonable default is folder-name (`/tables/foo.md` → `type: Table`) with a fallback like `type: Note`. The mechanism is small; the design decision is just to commit that bare-markdown is permitted internally and synthesised on export.

### 2. Reserved filenames

OKF reserves `index.md` and `log.md` at any directory level. memento today does not reserve these — a user could author `index.md` as a regular concept note.

If OKF compatibility ever becomes a stated goal, memento would need to either (a) treat `index.md`/`log.md` as reserved everywhere, breaking adoption of any existing vault that uses them as content, or (b) reserve them only in OKF-export mode, which means the export step rejects/renames or special-cases such files. Option (b) is the lower-friction choice and is consistent with how we already handle namespaces (`.memento/`, `_memento/`). The cost of (a) is that *every* memento vault becomes more constrained for the sake of a feature most users will not use. Lean (b).

### 3. Wikilinks vs. markdown links

OKF specifies standard markdown links (`[text](/path.md)` absolute, `[text](./other.md)` relative). memento's authoring surface is Obsidian's `[[wikilinks]]`, which we resolve at compile time (spec §4 link graph; §7 read).

This is mostly an export-side translation: at OKF export, wikilinks resolve to standard markdown links with `/`-prefixed absolute paths. The human authoring experience stays Obsidian-native; the OKF-conformant artifact is generated. The one wrinkle: OKF's "links MUST tolerate brokenness" combined with our resolved-vs-unresolved distinction means unresolved wikilinks at export time become broken markdown links pointing at non-existent paths. That is OKF-conformant (broken links are permitted) but worth being explicit about — we are not silently dropping unresolved links, we are emitting them as broken targets, which is the OKF-correct behaviour.

## Should we expose this as an explicit mode?

Two ways to think about it:

**Option A — single internal model, OKF as an export target.** memento stores and operates on its native model (vault-relative keys with `.md`, wikilinks, manifest, write modes). `memento export --okf <dir>` produces an OKF-conformant bundle as a one-shot artifact. The human keeps the Obsidian-native authoring surface; the OKF bundle is a publication step.

**Option B — dual-mode vault.** memento can operate on a vault that *is itself* an OKF bundle, with markdown links instead of wikilinks, `.md`-stripped IDs in cross-references, OKF-style `index.md`/`log.md` reserved. Probably toggled by a `format: okf` flag in `.memento/config.toml`.

Option A is much smaller. It treats OKF as an interchange format — the role OKF positions itself for — and keeps the internal model unconstrained. The Obsidian commitment is fully preserved because the human surface is unchanged. The export shim is bounded and testable.

Option B is bigger and pulls against the Obsidian commitment: Obsidian's wikilink rewriting, backlinks pane, and graph view are first-class for `[[wikilinks]]` and noticeably less polished for standard markdown links. Operating on an OKF-native vault degrades the human surface to gain a property (on-disk conformance) that Option A delivers via a one-step export. The only reason to want Option B is if memento is being used as a long-running editor over a vault that other OKF consumers also want to edit live — a use case we have no evidence for.

**Working position: Option A.** Pin "OKF as export target, not native mode" as the default posture. Revisit only if a concrete use case for live OKF-native editing surfaces.

A small contingent design move that costs nothing now: keep the manifest schema and the writing/read APIs export-agnostic. Specifically, do not let typed-link syntax leak into the markdown body (frontmatter only). That preserves Option A indefinitely.

## Where OKF is going, and whether we can be on the path early

The spec is explicit that v0.1 is "intentionally a starting point, designed for backward-compatible growth." A few directions look likely based on what is and is not in v0.1:

- **Compiled / cached indexes.** OKF's progressive-disclosure mechanism today is a tree of `index.md` files, manually curated or tool-generated. This is the wiki-folder-with-a-README pattern. It does not give a consumer a single scannable surface of titles + summaries + headings + tags for the whole bundle. The moment OKF bundles get large or are consumed by latency-sensitive agents, a compiled index will emerge — either as a convention (`/.okf/index.json`) or as a sibling artifact. memento's `manifest.json` is already that artifact. The honest claim: if OKF moves toward a compiled index, the shape of memento's manifest is a reasonable starting reference, and shipping `<bundle>/.okf/index.json` alongside the OKF bundle in our export is a low-cost contribution.
- **Section-anchored addressing.** OKF concept IDs are file paths. Section-level read addressing (memento's `read <key>#<heading>`) is not in v0.1. As soon as bundles include long-form concepts (specs, runbooks), section addressing becomes obvious. memento has worked through the heading-extraction and slug rules already (spec §7); these are publishable as a profile or extension proposal.
- **Typed links.** OKF's untyped-link rule is a v0.1 conservatism, not a design conviction (the spec acknowledges links assert *some* relationship). The hard cases — supersession between ADRs, depends-on between specs, embeds vs. references — will pressure-test the untyped rule fast. memento's frontmatter-encoded typed-link approach is one of the cleaner ways to add types without breaking the "body links are untyped" rule, because the body still parses identically under v0.1 consumers.
- **Write semantics / mutation.** OKF v0.1 is read-only in spirit — it specifies how to produce and consume, not how to update. Once enrichment agents start writing back, something like memento's write-mode taxonomy (`append-only`, `section-replace`, `keyed-upsert`, `read-only`) becomes load-bearing — agents will rationalise past prose rules. ADR-0015 is upstream of where OKF will land here.
- **Distribution and versioning.** OKF lists git-repo / tarball / zip / subdirectory. memento's "subdirectory in a larger repo" is the default case. If OKF develops conventions around package indexing or registry-like distribution, that is downstream of where we operate; track but don't pre-build.

**Reasonable posture for getting on early without locking in:**

1. Keep the manifest schema versioned (already committed, spec §4 `schema_version`) and treat it as something we might publish as a reference for an OKF-aligned index format.
2. Hold the line that typed-link information lives in frontmatter / the manifest, not in markdown body link syntax. Cheap to maintain; preserves OKF body-link conformance forever.
3. Avoid baking memento-specific filename conventions into the *human-visible vault root* where OKF will eventually reserve names. `_memento/` and `.memento/` are safely-prefixed; do not introduce a memento-flavoured `index.md` or `log.md` at the vault root.
4. When an OKF spec discussion opens (issues on the knowledge-catalog repo, follow-up posts), bring observations from dogfooding: how often section addressing is needed, where the untyped-link rule pinches, what the manifest is actually used for. Evidence beats speculation in standards discussions.
5. Defer the export shim itself until there is a concrete consumer asking for it. Cost of building it speculatively is real; cost of *preserving the ability to build it cheaply* is zero if the above constraints hold.

## What this is explicitly not

- **Not a commitment to OKF conformance.** memento's design center is the human-authored, agent-consumed in-repo memory substrate; OKF is one of several formats it could speak. If OKF stalls or diverges, this analysis loses force; the constraints above (typed-links-in-frontmatter, manifest versioning, namespace prefixes) are cheap insurance, not a strategic bet.
- **Not a roadmap change.** No v0–v4 entry needs adjustment based on this note. The export shim, if built, is post-v4 unless evidence pulls it forward.
- **Not a proposal to alter the Obsidian-as-human-interface commitment.** Everything above preserves it.

## Why this stays unresolved

OKF is one day old (as of writing). The spec is v0.1, explicitly a starting point. The right move is to record the alignment analysis and pin the cheap constraints (typed-link placement, namespace prefixing, manifest versioning) — not to build the export shim or expose a dual-mode flag before we know whether OKF gains traction.

Revisit triggers:

- An OKF-compatible consumer (BI tool, agent framework, catalog) emerges that we would want memento vaults to feed.
- An OKF spec discussion opens on compiled indexes, section addressing, typed links, or write semantics — direct opportunity to bring memento's prior art.
- A memento user requests OKF export specifically.
- OKF v0.2+ lands and either narrows or broadens the gap.

## Addendum 2026-06-14: dual-mode init as a deployment option

The framing above treats Option B (OKF-native vault) as "degrading the Obsidian surface." That framing is wrong for users who were not going to use Obsidian in the first place. The Obsidian commitment in spec §3 is an *opinionated default* for the typical deployment, not a constraint on the design space. A user whose human surface is VS Code, a static-site renderer, a web viewer, or no human surface at all (pure agent consumption) gets no benefit from wikilinks and is actively penalised by them when those tools render plain markdown.

So the question is better stated: **should `memento init` accept a `--format=okf` (or similar) flag that selects an OKF-native deployment, preserved in `.memento/config.toml`?** The Obsidian-default deployment is unaffected; the question is whether to support a second, equally first-class deployment shape.

### Use cases that would want this

- A team that already publishes OKF bundles externally wants its in-repo agent memory in the same format — no export step, no two-source drift.
- A non-Obsidian solo developer who prefers standard markdown links and gets better tooling support for them across editors and renderers.
- An organisation standardised on OKF as the cross-system interchange wants memento as the *editing* layer over those bundles, not just an *export* producer.

These are real, not hypothetical — though we have zero of them as users today.

### Costs of dual-mode that the export-only framing dodges

Building a `format:` toggle is not free, and the cost is structural, not one-off:

1. **Link-syntax has two paths through compile.** Wikilink resolution vs. standard markdown link resolution; different target-extraction rules; different what-counts-as-an-edge logic. Bounded but real.
2. **Key shape has two paths.** `.md`-suffixed vs `.md`-stripped concept IDs propagate through manifest, brief, read, write, error messages.
3. **Reserved-filename handling diverges.** OKF mode must treat `index.md`/`log.md` as operational, not concepts. Obsidian mode lets them be concepts.
4. **Write-mode metadata becomes a shared-vault hazard.** In OKF mode, memento's `mode:` frontmatter is an extension key — OKF-conformant, but another OKF tool editing the same bundle won't honour it. The "physical unwritability" property of `mode: read-only` (spec §8) weakens when the bundle is not exclusively memento-managed.
5. **Test surface doubles** for any path that branches on format.
6. **Every future design decision now has a two-axis answer.** This is the deepest cost — once `format` exists in config, typed-link encoding, summary rendering, link-graph computation, the brief layout, and any new verb has to decide what it does in each mode. Optionality has gravity.

Cost (6) is the one to weigh hardest. A toggle introduced casually accretes branches faster than the team carrying it can keep coherent.

### What we can do now without building it

The lowest-cost move is to preserve the *internal model's ability* to accommodate either deployment, without exposing the choice as a user-facing flag yet. Concretely:

- Keep the manifest schema agnostic to link *syntax* — manifest stores resolved keys and link types, not raw wikilink text. (Already the case per spec §4; worth pinning explicitly.)
- Localise key-shape decisions to a small number of call sites, so a future format toggle can change "`.md` stripped or not" in one place rather than threading through the code.
- Hold the line that typed-link information lives in frontmatter / manifest, never in body link syntax (already pinned above; the same constraint serves dual-mode and export-only equally).
- Do not introduce a memento-flavoured `index.md` or `log.md` at the vault root, in any mode. Those filenames belong to OKF in any future where dual-mode matters.

These cost nothing today. They preserve the option indefinitely.

### Revised working position

The earlier position — "Option A: OKF as export target, not native mode" — stands as the *default* deployment. The revision is that Option B is not rejected on principle; it is **deferred and pre-positioned**. If a user surfaces with the use case above, the path from "we don't do this" to "we do this" should not require a rewrite. The cheap constraints in this section are what keep that path open.

Trigger for actual implementation work: a concrete user with a non-Obsidian deployment intent, *or* a credible OKF consumer ecosystem where bidirectional editing (not just export) is the natural shape.

## Addendum 2026-06-26: reading the published example bundles

The original note (and its first addendum) was written from the SPEC alone, before Google published worked examples. The `knowledge-catalog` repo now ships three ready-to-browse bundles (`bundles/ga4`, `bundles/stackoverflow`, `bundles/crypto_bitcoin`), each produced by the POC enrichment agent, plus a per-bundle `viz.html` graph viewer. Reading them changes one concrete export decision and adds evidence to two open questions. The strategic posture (Option A default, Option B deferred-and-pre-positioned) is **unchanged**.

### Correction: export emits RELATIVE links, not `/`-absolute

This supersedes the export prescription in the "Wikilinks vs. markdown links" section above (the line stating wikilinks resolve "to standard markdown links with `/`-prefixed absolute paths"). That was inferred from SPEC §5.1, which *recommends* the absolute bundle-relative form (`[x](/tables/customers.md)`) "because it is stable when documents are moved." Two findings overturn it:

1. **None of the published bundles use the `/`-absolute form.** Grep across all three: zero leading-slash links. They use relative links exclusively — `[blocks](blocks.md)`, `[crypto_bitcoin](../datasets/crypto_bitcoin.md)`. Google's own tooling does not follow its own spec's recommendation.
2. **The `/`-absolute form breaks Obsidian.** A leading-slash path is *not* Obsidian's native "absolute path in vault" format (Obsidian's absolute form has no leading slash); Obsidian's resolver does not reliably bind it, so backlinks and click-through fail. Since the whole point of staying markdown-link-clean is to keep the Obsidian surface working, emitting `/`-absolute would defeat the goal.

**Pin for the export shim whenever it is built: emit relative links.** They are what the real bundles use *and* what Obsidian resolves. Follow the bundles, not the spec prose — the divergence between them is itself a signal that v0.1's written recommendations are aspirational and under-tested.

### Confirmed by the examples (no change, just now evidenced rather than assumed)

- **No wikilinks, no transclusion anywhere** in the bundles. Relationships are bare inline markdown links, untyped, relationship conveyed by surrounding prose — exactly as SPEC §5.3 states and as our typed-links-in-frontmatter constraint anticipated.
- **`type:` values are domain-qualified**, e.g. `BigQuery Table`, `BigQuery Dataset` — not bare `Table`. Our export-side `type:` synthesis default (folder-name → type) is roughly right but should lean toward a descriptive/qualified value, not a single-word folder name.
- **Filenames are `snake_case`, no spaces** — so the markdown-link space-encoding gotcha is moot for OKF-origin content. Worth preserving on our export side too (our slugs already are filename-safe, so wikilink→markdown-link is mechanical).
- **`index.md` is exactly the §6 shape** — a flat bulleted list of `[Title](url) - description` per section. Confirms it is a hand/tool-generated TOC, not a scannable whole-bundle surface.

### New evidence on "where OKF is going"

- **The compiled-index hypothesis (line ~80) is partially borne out, but the form differs.** Google felt the "single scannable surface" need and answered it with a per-bundle `viz.html` *graph viewer artifact* shipped alongside the markdown — not a `/.okf/index.json`. So the pressure toward a compiled consumption surface is real (good for the manifest-as-prior-art claim), but the ecosystem's first instinct was a human-facing viewer, not a machine-readable index. Our manifest still has no published OKF counterpart; the opening to propose one remains.
- **Spec-vs-practice divergence is a usable signal in any future standards discussion.** The tooling diverging from the spec on link form (and the spec being a v0.1 draft generally) means dogfooding evidence will carry weight — concrete "here is what the generator actually emits vs. what §5.1 says" observations are exactly the prior-art contributions point 4 of the early-posture list anticipated.

### Net

Only one prescription changes (relative links on export). Everything else the original note pinned — typed-links-in-frontmatter-only, namespace prefixes, manifest versioning, Option A as default — is reinforced, not revised, by the examples. No roadmap change. The clone reviewed lives at `/Users/tom/dev/personal/knowledge-catalog` (transient; not part of memento).

## Provenance

Originated in a 2026-06-14 conversation immediately after the Google Cloud OKF v0.1 announcement. Recorded to capture the alignment analysis while both formats are young and the cost of preserving compatibility is minimal. Addendum same day after the user pushed back on the "Obsidian as universal commitment" framing and surfaced the dual-mode deployment question. Second addendum 2026-06-26 after reviewing the published example bundles (`knowledge-catalog/okf/bundles/`), which corrected the link-form export prescription and added evidence on the compiled-index direction.

## Addendum 2026-06-29: memento as an OKF *producer*, and the deploy-an-agent-today *consumer* path

The note and its prior addenda treat OKF as an **interchange/export** concern — how a memento vault renders *out* to a conformant bundle (Option A, relative links, qualified `type:` synthesis, ratified subset). This addendum records two roles that analysis does not cover, surfaced in a 2026-06-29 conversation: (1) memento as a **producer** that *assembles* a well-connected vault from heterogeneous sources before rendering to OKF, and (2) the near-term shape of a **deployed-agent consumer** reading a packaged vault. The strategic posture above (Option A default) is unchanged; this extends it at both ends of the pipeline.

### 1. memento as a multi-source producer (generalising the reference enrichment agent)

Google's POC enrichment agent is a **single-structured-source crawler**: walk BigQuery metadata, emit OKF. The producer role memento is actually shaped for is the inverse on two axes — **many *unstructured* sources** (web, email, wikis) and **human-in-the-loop curation**. That is the differentiated producer position OKF's "format, not platform" punt deliberately leaves open. If GCP ships a hosted OKF store (see §3), the *consumer* side commoditises and the *curation/producer* side is where the moat is.

The **render half is already settled** (Option A and the link/type prescriptions above). The unsettled, higher-value half is the **gather → connect → ratify** pipeline:

- **"Well-connected" is the load-bearing phrase and does not come for free.** Naive multi-agent gathering across disparate sources yields *islands* — N internally-fine, graph-disconnected notes. Connectedness is the entire value (it is what turns a folder of dumps into a knowledge graph) and it takes real orchestration.
- memento has the right *primitives* (`brief`, the link graph, write modes) but **not the orchestration loop**. Required disciplines: **consult-brief-before-write** (a gathering agent reads `brief` + `inlinks/outlinks` *before* writing, so a finding attaches to and dedups against existing concepts rather than spawning a parallel island); the **compounding feedback loop** gather → read brief → link → write → recompile → richer brief for the next agent; **modes protecting the ratified core** mid-gather; and a **reconciliation/link pass** ("these three notes are one concept; this finding should link to that ADR").
- **Pin:** the gathering orchestration is *unbuilt* and is the actual product — larger than the export shim. The reconciliation pass is adjacent to the v4 agent-driven summarisation worklist; a **connectedness worklist** is its sibling.

### 2. Two governance gaps that only appear on gather-and-publish

The export analysis above is about *internal* authoring. Gathering externally and publishing org-wide crosses two boundaries it does not touch:

- **Provenance.** A note synthesised from an email thread or wiki page must carry origin + recency. This maps onto OKF's `resource:` and `timestamp:`. memento notes mostly do not populate these; for gathered knowledge it is *not* optional metadata — it is how a downstream consumer judges trust and staleness. **Cheap constraint:** give gathered notes a first-class `source:`/`resource:` field, which renders straight to OKF `resource:`.
- **Access-scope / PII.** The sharp one. What a gathering agent may *read* (internal email, private wiki) is not automatically fit to *publish* into an org-wide OKF store. OKF's permissive consumption model is silent here. This is exactly where [[conditional-information-access]]'s **deontic axis** (how access is *governed*, not just what the info *is*) becomes load-bearing: render needs an **eligibility filter**, not just a format transform. Rule shape: *do not export a concept whose sources sit above the target store's audience.*

**Clean resolution — ratification is the export gate.** Gathering agents produce unratified drafts freely; a human (or CI) reviews, connects, ratifies, and checks access-eligibility; render emits the **ratified, eligible subset**. This reuses the existing ratification boundary and invents no new mechanism — the boundary between "agents dumped stuff" and "fit to ship to the org."

### 3. Hosted OKF vault = roadmap validation

The likely arrival of a **first-class hosted GCP OKF store** turns the render/deploy-to-it path from speculative to roadmap-validated, and makes the producer pipeline its natural feed. Track it; do not pre-build the upload integration until the surface exists — consistent with the standing "defer the export shim until a concrete consumer asks" posture.

### 4. Deploying an ADK agent *today*: packaging a vault (retrieval-only)

Grounded in Agent Runtime's actual model (skill: `google-agents-cli-deploy`):

- **Packaging is source-based** — no Dockerfile; agent code ships as a **base64-encoded source tarball** (`uv export` → requirements; `deploy.py` packages source), Python pinned 3.12. **Arbitrary package data rides along in the tarball**, so a compiled bundle (notes + `.memento/manifest.json`) is shippable as co-located package data, read at cold start via a package-relative path. No filesystem mount needed.
- **The compile/read seam does the work.** `manifest.json` (stable, versioned, refuses unsupported `schema_version`) is the portable artifact; a deployed agent needs only a *reader* over manifest + notes — not the Go CLI, hooks, or write path.

**Packaging options:**

- **Co-located (today's default).** Bundle into the app package; the source tarball carries it; load once at `set_up()`/import. Zero network, version-pinned with the agent, git-rollback covers it. Cost: vault update ⇒ redeploy; tarball size; not shared across agents.
- **Remote (GCS — bridge to the hosted OKF store).** Ship a thin reader + a pointer; fetch `manifest.json` at cold start, lazy-fetch note bodies (LRU). Update without redeploy; shareable. Cost: cold-start latency, bucket IAM on the app SA, cache invalidation via GCS object generation/ETag. **GCS is reachable without VPC**; a *private* vault behind a VPC needs `--network-attachment` (PSC). GCS-backed remote is the low-friction path and generalises to a future hosted-OKF read.

**Reader = native Python over the manifest, not a shell-out.** Bundling/exec'ing the Go binary in a no-Dockerfile, fixed-Python managed runtime is fragile (arch, subprocess). The manifest *is* the contract — a small Python consumer projects `brief` and serves `read`. Guard CLI-drift by binding the reader to `manifest.schema_version` and conformance-testing both readers against one fixture vault.

**Tool shape — two `FunctionTool`s:** `memento_brief() -> dict` (manifest projection; orient/landscape) and `memento_read(key, heading=None) -> dict` (pull one note/section). Docstrings are the model-facing usage contract. This maps **1:1 onto an existing ADK idiom**: `PreloadMemoryTool` (inject at turn start) ≈ orient/brief; `LoadMemoryTool` (on-demand) ≈ read. Brief can be preloaded into the system instruction with **context caching** making it cheap.

**Crucial layering — do not conflate.** ADK's **Memory Bank / MemoryService is bottom-up, per-user, auto-generated episodic memory** ("what this user told me; facts learned across sessions," derived from conversation events). memento is **top-down, org-wide, human-curated, governed knowledge** ("what the project has ratified"). They are complementary layers — the memento tool sits *next to* `PreloadMemoryTool`, not instead of it. This is also the clean positioning line against memento being mistaken for ADK's native memory.

**Write story today: none.** No PreToolUse-hook or commit-boundary equivalent at runtime; retrieval-only is the correct v1. Deferred writeback (runtime append region → ratify back in-repo via the producer pipeline of §1) is a later path, not a today concern.

### Net / pins

- The **producer pipeline** (gather → connect → ratify) is the high-value *unbuilt* piece; render is the known small shim.
- Add a `source:`/`resource:` **provenance** field to gathered notes (cheap, OKF-aligned).
- An **access-eligibility filter** on export, keyed off [[conditional-information-access]]'s deontic axis; **ratification is the export + eligibility gate**.
- For **deploy-today**: co-located compiled bundle + native-Python manifest reader + `brief`/`read` `FunctionTool`s; `manifest.schema_version` is the consumer contract; retrieval-only.

**Revisit triggers (in addition to those above):** a hosted GCP OKF store ships; a concrete ADK deployment wants memento retrieval; demand surfaces for runtime writeback.

*Provenance: 2026-06-29 conversation extending the OKF analysis from interchange to (a) multi-source production and (b) deployed-agent (ADK / Agent Runtime) consumption. Deploy specifics grounded in the `google-agents-cli-deploy` / `-adk-code` skills. This addendum post-dates the Provenance section above.*
