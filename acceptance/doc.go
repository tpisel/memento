// Package acceptance holds memento's black-box, end-to-end acceptance suite for
// ADR-0031 (remove the write verb; hook-enforced native writes). It is the CI
// half of the validation gate described in the ADR's "Validation gate" section
// (§238-244 mechanical claims) and the memento-ryr epic plan's user stories
// US1-US12.
//
// Unlike the per-feature unit tests in internal/enforce, internal/cli, and
// internal/setup, this suite operates at integration altitude: it builds the
// real memento binary, runs `memento init` to install the hooks a user actually
// gets, then drives those installed PreToolUse / PostToolUse wrapper scripts and
// the real git hooks with synthesised harness payloads and git fixtures,
// asserting the verdicts. US13 (the read-only-leak-rate A/B on live agents) is
// deliberately NOT here — it is the manual, human-scored A-UAT run (ADR-0026,
// memento-ryr.22).
package acceptance
