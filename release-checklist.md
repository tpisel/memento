# v0.1.0 release checklist

Goal: get memento to a state where a friend who is not a Go developer can `brew install` it on macOS and try it on their own project without reading any ADRs.

## 1. Distribution: goreleaser + Homebrew tap

The standard path. One tag push builds binaries for all targets, generates checksums, cuts a GitHub Release, and writes a Homebrew formula into a tap repo automatically.

- [ ] **Create the shared tap repo manually:** `tpisel/homebrew-tap` on GitHub. Empty is fine; goreleaser will populate `Formula/memento.rb` on release. (Brew convention: a tap repo *must* be named `homebrew-<tapname>` and friends install with `brew install tpisel/tap/memento`. Using a generic `homebrew-tap` rather than a per-project `homebrew-memento` lets future tools reuse the same repo: each just adds `Formula/<name>.rb`.)
- [ ] **Mint a fine-grained PAT** with `contents:write` on the tap repo only. Store it as a secret on this repo: `HOMEBREW_TAP_GITHUB_TOKEN`. (Goreleaser uses this to push the formula update; the default `GITHUB_TOKEN` can't write to a different repo.)
- [ ] **Add `.goreleaser.yaml`** to this repo. Targets: darwin arm64+amd64, linux arm64+amd64. Windows optional — CI tests it, but friends asking for ergonomics are on Mac. Wire ldflags: `-X github.com/tpisel/memento/internal/cli.version={{.Version}}` so `memento version` reports the tag instead of `dev`. Include a `brews:` block pointing at `tpisel/homebrew-tap`.
- [ ] **Add `.github/workflows/release.yml`** — triggers on `v*` tag push, runs goreleaser with the tap token. Use `goreleaser/goreleaser-action@v6`.
- [ ] **Dry-run locally first:** `goreleaser release --snapshot --clean` builds the binaries without publishing. Catches config errors before they show up in CI.

## 2. Pre-flight (verify the install path actually works for a stranger)

- [ ] `go install github.com/tpisel/memento/cmd/memento@latest` works from a clean directory. (Quick check: `go install ./cmd/memento` in a scratch dir.) Keep this path working as the developer fallback.
- [ ] After ldflags wiring lands, `memento version` prints the tag (or a snapshot identifier in dev builds), not bare `dev`.
- [ ] `memento init` works in a brand-new git repo that has no `AGENTS.md`/`CLAUDE.md` yet. Empty-project path is the one friends will hit first.
- [ ] Smoke the README quickstart verbatim on a throwaway repo. The README is now a contract.

## 3. README / repo polish

- [ ] Update README install section: `brew install tpisel/tap/memento` first, `go install …` second, "grab a binary from the Releases page" third (covers Linux folks not on Linuxbrew).
- [ ] Skim the rewritten README for anything else to adjust.
- [ ] Decide whether to keep `Status: pre-1.0` framing or soften further. Current wording calls out manifest-schema breakage explicitly so people aren't surprised.
- [ ] Optional: add a `LICENSE` reference / badge — `LICENSE` exists but isn't called out in the README.
- [ ] Optional: one-paragraph "why not just dump everything into AGENTS.md" framing. Spec has the argument at §1; README currently elides it.

## 4. Release mechanics

- [ ] Tag `v0.1.0`: `git tag -a v0.1.0 -m "v0.1.0 — first taggable release"` then `git push origin v0.1.0`.
- [ ] The release workflow does the rest: builds binaries, attaches them to a GitHub Release, opens a PR (or direct commit) on the tap repo with the new formula.
- [ ] Verify on a clean machine: `brew install tpisel/tap/memento && memento version && memento init` in a scratch repo.
- [ ] Draft short GitHub Release notes pointing at the quickstart. Don't list every commit — link the README.

## 5. Explicitly out of scope for v0.1

- Homebrew core submission. Personal tap is enough.
- A `CHANGELOG` file. GitHub Release notes suffice at this stage.
- Windows binaries via brew. (Provide them in the Release for completeness if easy; nobody is `brew install`ing on Windows.)
- v4 features — agent-driven summarisation worklist, `review` verb, Obsidian-open.
- Docs site. Spec + ADRs in-repo remain canonical.
- **Cask migration (deferred to v0.2).** goreleaser's `brews` is hard-deprecated as of v2.16; the workflow is pinned to `~> v2.16` so it keeps working for v0.1. Migrating to `homebrew_casks` requires a post-install hook to strip `com.apple.quarantine` from the unsigned binary, or first-run is blocked by Gatekeeper — needs real testing on a clean Mac before switching.

## Manual steps summary (the bits goreleaser can't do for you)

1. Create the shared `tpisel/homebrew-tap` repo on GitHub.
2. Mint the fine-grained PAT and add it as `HOMEBREW_TAP_GITHUB_TOKEN`.
3. Push the `v0.1.0` tag once you're satisfied with a snapshot dry-run.

Everything else is files in this repo.
