package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// checkWritePayload renders a raw PreToolUse JSON payload for a file-targeted
// tool (Write/Edit/MultiEdit), mirroring the harness envelope check-write reads.
func checkWritePayload(t *testing.T, tool, filePath, content string) string {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"tool_name":  tool,
		"tool_input": map[string]any{"file_path": filePath, "content": content},
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return string(b)
}

// checkEditPayload renders a raw PreToolUse JSON payload for an Edit, mirroring
// the harness envelope: a single old_string→new_string substitution.
func checkEditPayload(t *testing.T, filePath, oldString, newString string, replaceAll bool) string {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"tool_name": "Edit",
		"tool_input": map[string]any{
			"file_path":   filePath,
			"old_string":  oldString,
			"new_string":  newString,
			"replace_all": replaceAll,
		},
	})
	if err != nil {
		t.Fatalf("marshal Edit payload: %v", err)
	}
	return string(b)
}

// invokeCheckWrite feeds payload on stdin and returns the verdict's decode plus the
// raw streams. A missing hookSpecificOutput (empty stdout) decodes to the zero
// value, which the allow/inert cases assert against.
func invokeCheckWrite(t *testing.T, payload string) (decision, reasonCode, reason, stdout, stderr string, code int) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = RunWithInput([]string{"check-write"}, strings.NewReader(payload), &out, &errOut)

	stdout, stderr = out.String(), errOut.String()
	if strings.TrimSpace(stdout) == "" {
		return "", "", "", stdout, stderr, code
	}
	var parsed struct {
		ReasonCode         string `json:"reason_code"`
		HookSpecificOutput struct {
			PermissionDecision       string `json:"permissionDecision"`
			PermissionDecisionReason string `json:"permissionDecisionReason"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
		t.Fatalf("check-write stdout = %q, not valid JSON: %v", stdout, err)
	}
	return parsed.HookSpecificOutput.PermissionDecision, parsed.ReasonCode, parsed.HookSpecificOutput.PermissionDecisionReason, stdout, stderr, code
}

func TestCheckWriteReadOnlyEditDenied(t *testing.T) { // US1
	root := makeCLIVault(t)
	initCLIGit(t, root)
	writeCLIFile(t, root, "frozen.md", "---\nmode: read-only\n---\n# Frozen\n\nOriginal.\n")
	commitCLIGit(t, root)

	target := filepath.Join(root, "frozen.md")
	decision, reasonCode, reason, _, stderr, code := invokeCheckWrite(t,
		checkWritePayload(t, "Write", target, "---\nmode: read-only\n---\n# Frozen\n\nRewritten.\n"))

	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %q", code, stderr)
	}
	if decision != "deny" {
		t.Fatalf("decision = %q, want deny", decision)
	}
	if reasonCode != "read_only" {
		t.Fatalf("reason_code = %q, want read_only", reasonCode)
	}
	for _, want := range []string{"frozen.md", "denied again", "memento unlock"} {
		if !strings.Contains(reason, want) {
			t.Fatalf("reason = %q, want it to contain %q", reason, want)
		}
	}
}

func TestCheckWriteAppendOnlyTruncateDeniedAppendAllowed(t *testing.T) { // US2
	root := makeCLIVault(t)
	initCLIGit(t, root)
	writeCLIFile(t, root, "log.md", "---\nmode: append-only\n---\n# Log\n\nEntry one.\n")
	commitCLIGit(t, root)
	target := filepath.Join(root, "log.md")
	old := "---\nmode: append-only\n---\n# Log\n\nEntry one.\n"

	// `>` / a truncating Write drops the tail: denied.
	decision, reasonCode, _, _, stderr, code := invokeCheckWrite(t,
		checkWritePayload(t, "Write", target, "---\nmode: append-only\n---\n# Log\n"))
	if code != 0 {
		t.Fatalf("truncate exit code = %d, want 0; stderr = %q", code, stderr)
	}
	if decision != "deny" || reasonCode != "append_only_overwrite" {
		t.Fatalf("truncate verdict = (%q,%q), want (deny, append_only_overwrite)", decision, reasonCode)
	}

	// `>>` / a Write that keeps the old bytes as a prefix: allowed.
	decision, _, _, _, stderr, code = invokeCheckWrite(t,
		checkWritePayload(t, "Write", target, old+"Entry two.\n"))
	if code != 0 {
		t.Fatalf("append exit code = %d, want 0; stderr = %q", code, stderr)
	}
	if decision != "allow" {
		t.Fatalf("append decision = %q, want allow", decision)
	}
}

func TestCheckWriteNewNoteAllowed(t *testing.T) { // US5
	root := makeCLIVault(t)
	initCLIGit(t, root)
	// No commit needed: the target does not exist, so it is creation, allowed
	// regardless of the mode it declares.

	target := filepath.Join(root, "fresh.md")
	decision, _, _, _, stderr, code := invokeCheckWrite(t,
		checkWritePayload(t, "Write", target, "---\nmode: read-only\n---\n# Fresh\n\nA brand-new frozen record.\n"))
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %q", code, stderr)
	}
	if decision != "allow" {
		t.Fatalf("decision = %q, want allow (new notes, including their mode, are created freely)", decision)
	}
}

func TestCheckWriteUnratifiedNoteAllowed(t *testing.T) { // US6
	root := makeCLIVault(t)
	initCLIGit(t, root)
	// Written but never committed: still inside its edit window.
	writeCLIFile(t, root, "draft.md", "---\nmode: read-only\n---\n# Draft\n\nFirst.\n")

	target := filepath.Join(root, "draft.md")
	decision, _, _, _, stderr, code := invokeCheckWrite(t,
		checkWritePayload(t, "Write", target, "---\nmode: read-only\n---\n# Draft\n\nReworked.\n"))
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %q", code, stderr)
	}
	if decision != "allow" {
		t.Fatalf("decision = %q, want allow (unratified notes accept any write)", decision)
	}
}

func TestCheckWriteActiveGrantReopensReadOnly(t *testing.T) {
	root := makeCLIVault(t)
	initCLIGit(t, root)
	writeCLIFile(t, root, "frozen.md", "---\nmode: read-only\n---\n# Frozen\n\nOriginal.\n")
	commitCLIGit(t, root)

	var ustdout, ustderr bytes.Buffer
	if c := Run([]string{"unlock", "frozen.md", "--justification", "fix typo"}, &ustdout, &ustderr); c != 0 {
		t.Fatalf("unlock exit code = %d, want 0; stderr = %q", c, ustderr.String())
	}

	target := filepath.Join(root, "frozen.md")
	decision, _, _, _, stderr, code := invokeCheckWrite(t,
		checkWritePayload(t, "Write", target, "---\nmode: read-only\n---\n# Frozen\n\nFixed.\n"))
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %q", code, stderr)
	}
	if decision != "allow" {
		t.Fatalf("decision = %q, want allow under an active unlock grant", decision)
	}
}

func TestCheckWriteDriveByModeChangeDeniedUnderGrant(t *testing.T) { // US4
	root := makeCLIVault(t)
	initCLIGit(t, root)
	writeCLIFile(t, root, "frozen.md", "---\nmode: read-only\n---\n# Frozen\n\nOriginal.\n")
	commitCLIGit(t, root)

	// Unlock re-opens the body, but the mode: field is never in scope.
	var ustdout, ustderr bytes.Buffer
	if c := Run([]string{"unlock", "frozen.md", "--justification", "fix typo"}, &ustdout, &ustderr); c != 0 {
		t.Fatalf("unlock exit code = %d, want 0; stderr = %q", c, ustderr.String())
	}

	target := filepath.Join(root, "frozen.md")
	// Same write smuggles a permanent mode: read-only → living under the grant.
	decision, reasonCode, reason, _, stderr, code := invokeCheckWrite(t,
		checkWritePayload(t, "Write", target, "---\nmode: living\n---\n# Frozen\n\nFixed.\n"))
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %q", code, stderr)
	}
	if decision != "deny" || reasonCode != "drive_by_mode_change" {
		t.Fatalf("verdict = (%q,%q), want (deny, drive_by_mode_change)", decision, reasonCode)
	}
	for _, want := range []string{"frozen.md", "denied again", "write-mode"} {
		if !strings.Contains(reason, want) {
			t.Fatalf("reason = %q, want it to contain %q", reason, want)
		}
	}

	// A body-only edit under the same grant (mode line untouched) is allowed:
	// the defense gates the mode field, not the unlock.
	decision, _, _, _, stderr, code = invokeCheckWrite(t,
		checkWritePayload(t, "Write", target, "---\nmode: read-only\n---\n# Frozen\n\nFixed.\n"))
	if code != 0 {
		t.Fatalf("body-only exit code = %d, want 0; stderr = %q", code, stderr)
	}
	if decision != "allow" {
		t.Fatalf("body-only decision = %q, want allow under an active grant", decision)
	}
}

func TestCheckWriteUnparseableFrontmatterDenied(t *testing.T) {
	root := makeCLIVault(t)
	initCLIGit(t, root)
	writeCLIFile(t, root, "note.md", "---\nmode: append-only\n---\n# Note\n\nBody.\n")
	commitCLIGit(t, root)

	target := filepath.Join(root, "note.md")
	// New bytes whose frontmatter block no longer parses (a line with no colon)
	// mean mode safety cannot be verified for a ratified note: denied.
	decision, reasonCode, _, _, stderr, code := invokeCheckWrite(t,
		checkWritePayload(t, "Write", target, "---\nmode: append-only\nthis frontmatter line has no colon\n---\n# Note\n\nBody.\n"))
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %q", code, stderr)
	}
	if decision != "deny" || reasonCode != "drive_by_mode_change" {
		t.Fatalf("verdict = (%q,%q), want (deny, drive_by_mode_change)", decision, reasonCode)
	}
}

func TestCheckWriteUnwritablePathDenied(t *testing.T) {
	root := makeCLIVault(t)
	target := filepath.Join(root, "_memento", "writing.md")

	decision, reasonCode, _, _, stderr, code := invokeCheckWrite(t,
		checkWritePayload(t, "Write", target, "anything"))
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %q", code, stderr)
	}
	if decision != "deny" || reasonCode != "unwritable_path" {
		t.Fatalf("verdict = (%q,%q), want (deny, unwritable_path)", decision, reasonCode)
	}
}

func TestCheckWriteOutsideVaultIsInert(t *testing.T) {
	root := makeCLIVault(t)
	// A sibling of the vault root, outside it.
	target := filepath.Join(filepath.Dir(root), "README.md")

	_, _, _, stdout, stderr, code := invokeCheckWrite(t,
		checkWritePayload(t, "Write", target, "anything"))
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %q", code, stderr)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Fatalf("stdout = %q, want empty (writes outside the vault are not gated)", stdout)
	}
}

func TestCheckWriteNonFileToolIsInert(t *testing.T) {
	makeCLIVault(t)
	b, err := json.Marshal(map[string]any{"tool_name": "Read", "tool_input": map[string]any{"file_path": "/tmp/x"}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	_, _, _, stdout, stderr, code := invokeCheckWrite(t, string(b))
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %q", code, stderr)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Fatalf("stdout = %q, want empty for a non-write tool", stdout)
	}
}

func TestCheckWriteAmbiguousVaultAsks(t *testing.T) {
	repo := t.TempDir()
	for _, sub := range []string{"alpha", "beta"} {
		if err := os.MkdirAll(filepath.Join(repo, sub, ".memento"), 0o755); err != nil {
			t.Fatalf("mkdir %s marker: %v", sub, err)
		}
	}
	chdirCLI(t, repo)

	target := filepath.Join(repo, "alpha", "note.md")
	decision, reasonCode, _, _, stderr, code := invokeCheckWrite(t,
		checkWritePayload(t, "Write", target, "x"))
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %q", code, stderr)
	}
	if decision != "ask" || reasonCode != "vault_discovery_ambiguous" {
		t.Fatalf("verdict = (%q,%q), want (ask, vault_discovery_ambiguous)", decision, reasonCode)
	}
}

func TestCheckWriteAppendOnlyInteriorEditDeniedTailAppendAllowed(t *testing.T) { // US3
	root := makeCLIVault(t)
	initCLIGit(t, root)
	writeCLIFile(t, root, "log.md", "---\nmode: append-only\n---\n# Log\n\nEntry one.\nEntry two.\n")
	commitCLIGit(t, root)
	target := filepath.Join(root, "log.md")

	// An interior Edit rewrites bytes the old content already committed, breaking
	// the append-only prefix: denied.
	decision, reasonCode, reason, _, stderr, code := invokeCheckWrite(t,
		checkEditPayload(t, target, "Entry one.", "Edited one.", false))
	if code != 0 {
		t.Fatalf("interior edit exit code = %d, want 0; stderr = %q", code, stderr)
	}
	if decision != "deny" || reasonCode != "append_only_interior" {
		t.Fatalf("interior edit verdict = (%q,%q), want (deny, append_only_interior)", decision, reasonCode)
	}
	if !strings.Contains(reason, "log.md") {
		t.Fatalf("reason = %q, want it to name the note", reason)
	}

	// A tail-append Edit extends the last line, keeping the old bytes as a prefix:
	// allowed.
	decision, _, _, _, stderr, code = invokeCheckWrite(t,
		checkEditPayload(t, target, "Entry two.\n", "Entry two.\nEntry three.\n", false))
	if code != 0 {
		t.Fatalf("tail append exit code = %d, want 0; stderr = %q", code, stderr)
	}
	if decision != "allow" {
		t.Fatalf("tail append decision = %q, want allow", decision)
	}
}

func TestCheckWriteUnderivableEditFailsClosed(t *testing.T) {
	root := makeCLIVault(t)
	initCLIGit(t, root)
	writeCLIFile(t, root, "note.md", "---\nmode: read-only\n---\n# Note\n\nBody.\n")
	commitCLIGit(t, root)

	target := filepath.Join(root, "note.md")
	// old_string is absent from the note, so the replay aborts and no faithful
	// new-bytes exist: the wrapper must fail closed rather than gate invented bytes.
	_, _, _, _, stderr, code := invokeCheckWrite(t,
		checkEditPayload(t, target, "no such text", "x", false))
	if code == 0 {
		t.Fatalf("exit code = 0, want non-zero for an in-vault edit whose replay aborts (wrapper fails closed)")
	}
	if !strings.Contains(stderr, "Edit") {
		t.Fatalf("stderr = %q, want it to name the tool", stderr)
	}
}

// applyPatchPayload renders a raw PreToolUse JSON payload for a codex apply_patch
// call, carrying the envelope under the untyped `input` key (codex hooks contract).
func applyPatchPayload(t *testing.T, patch string) string {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"tool_name":  "apply_patch",
		"tool_input": map[string]any{"input": patch},
	})
	if err != nil {
		t.Fatalf("marshal apply_patch payload: %v", err)
	}
	return string(b)
}

func TestCheckWriteApplyPatchReadOnlyUpdateDenied(t *testing.T) { // US12
	root := makeCLIVault(t)
	initCLIGit(t, root)
	writeCLIFile(t, root, "frozen.md", "---\nmode: read-only\n---\n# Frozen\n\nOriginal.\n")
	commitCLIGit(t, root)
	target := filepath.Join(root, "frozen.md")

	patch := strings.Join([]string{
		"*** Begin Patch",
		"*** Update File: " + target,
		"@@",
		" # Frozen",
		" ",
		"-Original.",
		"+Rewritten.",
		"*** End Patch",
	}, "\n")

	decision, reasonCode, reason, codexStdout, stderr, code := invokeCheckWrite(t, applyPatchPayload(t, patch))
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %q", code, stderr)
	}
	if decision != "deny" || reasonCode != "read_only" {
		t.Fatalf("verdict = (%q,%q), want (deny, read_only)", decision, reasonCode)
	}
	for _, want := range []string{"frozen.md", "denied again", "memento unlock"} {
		if !strings.Contains(reason, want) {
			t.Fatalf("reason = %q, want it to contain %q", reason, want)
		}
	}

	// The deny envelope must be byte-identical to the Claude contract: the same
	// read-only Write deny produces the same stdout (ADR-0031, codex hooks contract).
	_, _, _, claudeStdout, _, _ := invokeCheckWrite(t,
		checkWritePayload(t, "Write", target, "---\nmode: read-only\n---\n# Frozen\n\nRewritten.\n"))
	if codexStdout != claudeStdout {
		t.Fatalf("apply_patch deny stdout not byte-identical to Claude Write deny:\n codex  = %q\n claude = %q", codexStdout, claudeStdout)
	}
}

func TestCheckWriteApplyPatchAppendOnlyInteriorDeniedTailAllowed(t *testing.T) {
	root := makeCLIVault(t)
	initCLIGit(t, root)
	writeCLIFile(t, root, "log.md", "---\nmode: append-only\n---\n# Log\n\nEntry one.\nEntry two.\n")
	commitCLIGit(t, root)
	target := filepath.Join(root, "log.md")

	interior := strings.Join([]string{
		"*** Begin Patch",
		"*** Update File: " + target,
		"@@",
		"-Entry one.",
		"+Edited one.",
		"*** End Patch",
	}, "\n")
	decision, reasonCode, _, _, stderr, code := invokeCheckWrite(t, applyPatchPayload(t, interior))
	if code != 0 {
		t.Fatalf("interior exit code = %d, want 0; stderr = %q", code, stderr)
	}
	if decision != "deny" || reasonCode != "append_only_interior" {
		t.Fatalf("interior verdict = (%q,%q), want (deny, append_only_interior)", decision, reasonCode)
	}

	tail := strings.Join([]string{
		"*** Begin Patch",
		"*** Update File: " + target,
		"@@",
		" Entry two.",
		"+Entry three.",
		"*** End Patch",
	}, "\n")
	decision, _, _, _, stderr, code = invokeCheckWrite(t, applyPatchPayload(t, tail))
	if code != 0 {
		t.Fatalf("tail exit code = %d, want 0; stderr = %q", code, stderr)
	}
	if decision != "allow" {
		t.Fatalf("tail decision = %q, want allow", decision)
	}
}

func TestCheckWriteApplyPatchAddNewNoteAllowed(t *testing.T) {
	root := makeCLIVault(t)
	initCLIGit(t, root)
	target := filepath.Join(root, "fresh.md")

	patch := strings.Join([]string{
		"*** Begin Patch",
		"*** Add File: " + target,
		"+---",
		"+mode: read-only",
		"+---",
		"+# Fresh",
		"*** End Patch",
	}, "\n")
	decision, _, _, _, stderr, code := invokeCheckWrite(t, applyPatchPayload(t, patch))
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %q", code, stderr)
	}
	if decision != "allow" {
		t.Fatalf("decision = %q, want allow (new notes are created freely)", decision)
	}
}

func TestCheckWriteApplyPatchDeleteVaultNoteDenied(t *testing.T) {
	root := makeCLIVault(t)
	initCLIGit(t, root)
	writeCLIFile(t, root, "frozen.md", "---\nmode: read-only\n---\n# Frozen\n\nOriginal.\n")
	commitCLIGit(t, root)
	target := filepath.Join(root, "frozen.md")

	patch := "*** Begin Patch\n*** Delete File: " + target + "\n*** End Patch"
	decision, reasonCode, _, _, stderr, code := invokeCheckWrite(t, applyPatchPayload(t, patch))
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %q", code, stderr)
	}
	if decision != "deny" || reasonCode != "apply_patch_unsupported_op" {
		t.Fatalf("verdict = (%q,%q), want (deny, apply_patch_unsupported_op)", decision, reasonCode)
	}
}

func TestCheckWriteApplyPatchOutsideVaultIsInert(t *testing.T) {
	root := makeCLIVault(t)
	outside := filepath.Join(filepath.Dir(root), "README.md")

	patch := strings.Join([]string{
		"*** Begin Patch",
		"*** Update File: " + outside,
		"@@",
		"-a",
		"+b",
		"*** End Patch",
	}, "\n")
	_, _, _, stdout, stderr, code := invokeCheckWrite(t, applyPatchPayload(t, patch))
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %q", code, stderr)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Fatalf("stdout = %q, want empty (patches that touch no vault note are not gated)", stdout)
	}
}

func TestCheckWriteApplyPatchNoEnvelopeFailsClosed(t *testing.T) {
	makeCLIVault(t)
	b, err := json.Marshal(map[string]any{
		"tool_name":  "apply_patch",
		"tool_input": map[string]any{"input": "not a patch at all"},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	_, _, _, _, stderr, code := invokeCheckWrite(t, string(b))
	if code == 0 {
		t.Fatalf("exit code = 0, want non-zero for an apply_patch payload with no recognisable envelope")
	}
	if !strings.Contains(stderr, "envelope") {
		t.Fatalf("stderr = %q, want it to mention the missing envelope", stderr)
	}
}

func TestCheckWriteApplyPatchBareStringToolInput(t *testing.T) {
	root := makeCLIVault(t)
	initCLIGit(t, root)
	writeCLIFile(t, root, "frozen.md", "---\nmode: read-only\n---\n# Frozen\n\nOriginal.\n")
	commitCLIGit(t, root)
	target := filepath.Join(root, "frozen.md")

	patch := strings.Join([]string{
		"*** Begin Patch",
		"*** Update File: " + target,
		"@@",
		"-Original.",
		"+Rewritten.",
		"*** End Patch",
	}, "\n")
	// tool_input is the bare patch string rather than an object — findPatchEnvelope
	// must still recover the envelope.
	b, err := json.Marshal(map[string]any{"tool_name": "apply_patch", "tool_input": patch})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	decision, reasonCode, _, _, stderr, code := invokeCheckWrite(t, string(b))
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %q", code, stderr)
	}
	if decision != "deny" || reasonCode != "read_only" {
		t.Fatalf("verdict = (%q,%q), want (deny, read_only)", decision, reasonCode)
	}
}

func TestCheckWriteNotInTopLevelHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"help"}, &stdout, &stderr); code != 0 {
		t.Fatalf("Run(help) exit code = %d, want 0", code)
	}
	if strings.Contains(stdout.String(), "check-write") {
		t.Fatalf("help text lists check-write, but it is hook plumbing and must stay hidden:\n%s", stdout.String())
	}
}
