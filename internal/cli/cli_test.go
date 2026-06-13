package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHelpCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := Run([]string{"help"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(help) exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("Run(help) wrote stderr = %q, want empty", stderr.String())
	}

	out := stdout.String()
	for _, want := range []string{
		"memento",
		"Usage:",
		"brief",
		"compile",
		"read",
		"version",
		"serve     MCP server (not implemented; see spec §13).",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("Run(help) output %q does not contain %q", out, want)
		}
	}
}

func TestDefaultCommandShowsHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := Run(nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(nil) exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Usage:") {
		t.Fatalf("Run(nil) output %q does not contain Usage", stdout.String())
	}
}

func TestVersionCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := Run([]string{"version"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(version) exit code = %d, want 0; stderr = %q", code, stderr.String())
	}

	got := strings.TrimSpace(stdout.String())
	if got != "memento dev" {
		t.Fatalf("Run(version) output = %q, want %q", got, "memento dev")
	}
}

func TestUnknownCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := Run([]string{"bogus"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("Run(bogus) exit code = %d, want 2", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("Run(bogus) wrote stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), `unknown command "bogus"`) {
		t.Fatalf("Run(bogus) stderr = %q, want unknown command message", stderr.String())
	}
}

func TestServeCommandIsFutureStub(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := Run([]string{"serve"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("Run(serve) exit code = %d, want 1", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("Run(serve) wrote stdout = %q, want empty", stdout.String())
	}
	for _, want := range []string{"not implemented", "v3", "spec §13"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("Run(serve) stderr = %q, want %q", stderr.String(), want)
		}
	}
	if strings.Contains(stderr.String(), "unknown command") {
		t.Fatalf("Run(serve) stderr = %q, want non-usage not-implemented error", stderr.String())
	}
}

func TestInitCreatesDefaultVaultForEmptyProject(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "sample-app")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	chdirCLI(t, repo)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"init"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(init) exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("Run(init) stderr = %q, want empty", stderr.String())
	}

	root := filepath.Join(repo, "sample-app-memory")
	for _, relPath := range []string{
		".memento",
		".memento/config.toml",
		".memento/manifest.json",
		".mementoignore",
		"example.md",
	} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(relPath))); err != nil {
			t.Fatalf("stat %s: %v", relPath, err)
		}
	}

	manifest := readCLIFile(t, root, ".memento/manifest.json")
	if !strings.Contains(manifest, `"key": "example.md"`) {
		t.Fatalf("manifest = %q, want example note entry", manifest)
	}
	if !strings.Contains(stdout.String(), root) {
		t.Fatalf("Run(init) stdout = %q, want initialized vault path %q", stdout.String(), root)
	}
}

func TestInitAdoptsNonEmptyExplicitDir(t *testing.T) {
	repo := t.TempDir()
	root := filepath.Join(repo, "existing-memory")
	writeCLIFile(t, root, "note.md", "# Adopted\n\nExisting note.\n")
	chdirCLI(t, repo)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"init", "--dir", "existing-memory"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(init --dir existing-memory) exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if stdout.Len() == 0 {
		t.Fatalf("Run(init --dir existing-memory) stdout = empty, want initialized path")
	}
	if stderr.Len() != 0 {
		t.Fatalf("Run(init --dir existing-memory) stderr = %q, want empty", stderr.String())
	}
	if got := readCLIFile(t, root, "note.md"); got != "# Adopted\n\nExisting note.\n" {
		t.Fatalf("adopted note changed to %q", got)
	}
	if _, err := os.Stat(filepath.Join(root, "example.md")); !os.IsNotExist(err) {
		t.Fatalf("example.md stat err = %v, want file not to exist for adopted vault", err)
	}

	manifest := readCLIFile(t, root, ".memento/manifest.json")
	if !strings.Contains(manifest, `"key": "note.md"`) {
		t.Fatalf("manifest = %q, want adopted note entry", manifest)
	}
}

func TestInitDoesNotClobberExistingFilesystemArtifacts(t *testing.T) {
	repo := t.TempDir()
	root := filepath.Join(repo, "existing-memory")
	writeCLIFile(t, root, ".memento/config.toml", "custom config\n")
	writeCLIFile(t, root, ".memento/manifest.json", "custom manifest\n")
	writeCLIFile(t, root, ".mementoignore", "custom ignore\n")
	chdirCLI(t, repo)

	for i := 0; i < 2; i++ {
		var stdout, stderr bytes.Buffer
		code := Run([]string{"init", "--dir", "existing-memory"}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("Run(init --dir existing-memory) iteration %d exit code = %d, want 0; stderr = %q", i+1, code, stderr.String())
		}
		if stderr.Len() != 0 {
			t.Fatalf("Run(init --dir existing-memory) iteration %d stderr = %q, want empty", i+1, stderr.String())
		}
	}

	if got := readCLIFile(t, root, ".memento/config.toml"); got != "custom config\n" {
		t.Fatalf("config clobbered: %q", got)
	}
	if got := readCLIFile(t, root, ".memento/manifest.json"); got != "custom manifest\n" {
		t.Fatalf("manifest clobbered: %q", got)
	}
	if got := readCLIFile(t, root, ".mementoignore"); !strings.HasPrefix(got, "custom ignore\n") || !strings.Contains(got, "_memento/brief.md") {
		t.Fatalf("ignore update = %q, want existing content preserved and brief entry added", got)
	}
}

func TestCompilePrintsManifestForExplicitDir(t *testing.T) {
	root := makeCLIVault(t)
	writeCLIFile(t, root, "note.md", "# Note\n\nSummary.\n")

	var stdout, stderr bytes.Buffer
	code := Run([]string{"compile", "--dir", root, "--print"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(compile --print) exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("Run(compile --print) stderr = %q, want empty", stderr.String())
	}
	if !strings.Contains(stdout.String(), `"key": "note.md"`) {
		t.Fatalf("Run(compile --print) stdout = %q, want note entry", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(root, ".memento", "manifest.json")); !os.IsNotExist(err) {
		t.Fatalf("Run(compile --print) wrote manifest unexpectedly; stat err = %v", err)
	}
}

func TestCompilePrintWarnsForMalformedFrontmatter(t *testing.T) {
	root := makeCLIVault(t)
	writeCLIFile(t, root, "broken.md", `---
title
---
# Fallback

Summary.
`)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"compile", "--dir", root, "--print"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(compile --print) exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"title": "Fallback"`) {
		t.Fatalf("Run(compile --print) stdout = %q, want fallback title", stdout.String())
	}
	errOut := stderr.String()
	for _, want := range []string{"warning", "broken.md", "malformed frontmatter"} {
		if !strings.Contains(errOut, want) {
			t.Fatalf("Run(compile --print) stderr = %q, want %q", errOut, want)
		}
	}
}

func TestCompileWritesDiscoveredManifest(t *testing.T) {
	repo := t.TempDir()
	root := filepath.Join(repo, "project-memory")
	if err := os.MkdirAll(filepath.Join(root, ".memento"), 0o755); err != nil {
		t.Fatalf("mkdir marker: %v", err)
	}
	writeCLIFile(t, root, "note.md", "# Note\n\nSummary.\n")

	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(repo); err != nil {
		t.Fatalf("chdir repo: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previous); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"compile"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(compile) exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("Run(compile) stdout = %q, want empty", stdout.String())
	}

	manifestPath := filepath.Join(root, ".memento", "manifest.json")
	contents, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if !strings.Contains(string(contents), `"key": "note.md"`) {
		t.Fatalf("manifest contents = %q, want note entry", string(contents))
	}

	briefPath := filepath.Join(root, "_memento", "brief.md")
	brief, err := os.ReadFile(briefPath)
	if err != nil {
		t.Fatalf("read brief: %v", err)
	}
	for _, want := range []string{
		"<!-- manifest: sha256:",
		"mode: read-only",
		"> [!caution] Auto-generated by `memento compile`",
		"> Any edits to this file will be overwritten on the next compile run.",
		"## (root)",
		"### 1. Note",
		"key: `note.md`",
	} {
		if !strings.Contains(string(brief), want) {
			t.Fatalf("brief contents = %q, want %q", string(brief), want)
		}
	}
}

func TestCompileWritesManifestAndWarnsWhenBriefWriteFails(t *testing.T) {
	root := makeCLIVault(t)
	writeCLIFile(t, root, "note.md", "# Note\n\nSummary.\n")
	writeCLIFile(t, root, "_memento", "not a directory\n")

	var stdout, stderr bytes.Buffer
	code := Run([]string{"compile", "--dir", root}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(compile) exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("Run(compile) stdout = %q, want empty", stdout.String())
	}

	manifestPath := filepath.Join(root, ".memento", "manifest.json")
	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if !strings.Contains(string(manifest), `"key": "note.md"`) {
		t.Fatalf("manifest contents = %q, want note entry", string(manifest))
	}
	for _, want := range []string{"warning", "_memento/brief.md"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("Run(compile) stderr = %q, want %q", stderr.String(), want)
		}
	}
}

func TestCompileArtifactsDoNotLeakHTMLEscapedSequences(t *testing.T) {
	root := makeCLIVault(t)
	writeCLIFile(t, root, "angle.md", `---
title: Angle <Tag>
summary: Summary has <, >, and &.
tags: [a&b]
---

# Angle

Body.
`)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"compile", "--dir", root}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(compile) exit code = %d, want 0; stderr = %q", code, stderr.String())
	}

	for _, relPath := range []string{".memento/manifest.json", "_memento/brief.md"} {
		data := readCLIFile(t, root, relPath)
		if strings.Contains(data, "\\u") {
			t.Fatalf("%s contains escaped unicode sequence:\n%s", relPath, data)
		}
	}
}

func TestBriefPrintsExistingBriefForExplicitDir(t *testing.T) {
	root := makeCLIVault(t)
	want := "# Existing Brief\n\nAlready rendered.\n"
	writeCLIFile(t, root, "_memento/brief.md", want)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"brief", "--dir", root}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(brief --dir) exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("Run(brief --dir) stderr = %q, want empty", stderr.String())
	}
	if stdout.String() != want {
		t.Fatalf("Run(brief --dir) stdout = %q, want %q", stdout.String(), want)
	}
}

func TestBriefRendersFromManifestWhenBriefIsMissing(t *testing.T) {
	root := makeCLIVault(t)
	writeCLIFile(t, root, "note.md", "# Note\n\nSummary.\n")

	var compileStdout, compileStderr bytes.Buffer
	code := Run([]string{"compile", "--dir", root}, &compileStdout, &compileStderr)
	if code != 0 {
		t.Fatalf("Run(compile) exit code = %d, want 0; stderr = %q", code, compileStderr.String())
	}
	briefPath := filepath.Join(root, "_memento", "brief.md")
	if err := os.Remove(briefPath); err != nil {
		t.Fatalf("remove brief: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code = Run([]string{"brief", "--dir", root}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(brief --dir) exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("Run(brief --dir) stderr = %q, want empty", stderr.String())
	}
	for _, want := range []string{
		"<!-- manifest: sha256:",
		"mode: read-only",
		"> [!caution] Auto-generated by `memento compile`",
		"> Any edits to this file will be overwritten on the next compile run.",
		"## (root)",
		"### 1. Note",
		"key: `note.md`",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("Run(brief --dir) stdout = %q, want %q", stdout.String(), want)
		}
	}
	if got := readCLIFile(t, root, "_memento/brief.md"); got != stdout.String() {
		t.Fatalf("written brief = %q, want stdout %q", got, stdout.String())
	}
}

func TestBriefFailsWithCompileHintWhenArtifactsAreMissing(t *testing.T) {
	root := makeCLIVault(t)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"brief", "--dir", root}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("Run(brief --dir) exit code = %d, want 1", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("Run(brief --dir) stdout = %q, want empty", stdout.String())
	}
	for _, want := range []string{"manifest", "run memento compile"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("Run(brief --dir) stderr = %q, want %q", stderr.String(), want)
		}
	}
}

func TestReadPrintsRequestedMarkdownForExplicitDir(t *testing.T) {
	root := makeCLIVault(t)
	writeCLIFile(t, root, "notes/deep.md", "# Deep\n\nNested content.\n")

	var stdout, stderr bytes.Buffer
	code := Run([]string{"read", "--dir", root, "notes/deep.md"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(read --dir notes/deep.md) exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("Run(read) stderr = %q, want empty", stderr.String())
	}

	want := "# Deep\n\nNested content.\n"
	if stdout.String() != want {
		t.Fatalf("Run(read) stdout = %q, want %q", stdout.String(), want)
	}
}

func TestReadNumericReferenceResolvesAgainstManifestOrdering(t *testing.T) {
	root := makeCLIVault(t)
	writeCLIFile(t, root, "zeta.md", "# Zeta\n\nRoot note.\n")
	writeCLIFile(t, root, "notes/beta.md", "# Beta\n\nNested note.\n")

	var compileStdout, compileStderr bytes.Buffer
	code := Run([]string{"compile", "--dir", root}, &compileStdout, &compileStderr)
	if code != 0 {
		t.Fatalf("Run(compile) exit code = %d, want 0; stderr = %q", code, compileStderr.String())
	}

	var stdout, stderr bytes.Buffer
	code = Run([]string{"read", "--dir", root, "2"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(read 2) exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("Run(read 2) stderr = %q, want empty", stderr.String())
	}
	if want := "# Beta\n\nNested note.\n"; stdout.String() != want {
		t.Fatalf("Run(read 2) stdout = %q, want %q", stdout.String(), want)
	}
}

func TestReadNumericReferenceFailsWithStaleManifestMessageForMissingFile(t *testing.T) {
	root := makeCLIVault(t)
	writeCLIFile(t, root, "note.md", "# Note\n\nSummary.\n")

	var compileStdout, compileStderr bytes.Buffer
	code := Run([]string{"compile", "--dir", root}, &compileStdout, &compileStderr)
	if code != 0 {
		t.Fatalf("Run(compile) exit code = %d, want 0; stderr = %q", code, compileStderr.String())
	}
	if err := os.Remove(filepath.Join(root, "note.md")); err != nil {
		t.Fatalf("remove note.md: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code = Run([]string{"read", "--dir", root, "1"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("Run(read 1) exit code = %d, want 1", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("Run(read 1) stdout = %q, want empty", stdout.String())
	}
	for _, want := range []string{
		"entry 1's file `note.md` no longer exists.",
		"manifest is stale; run: memento compile && memento brief",
		"note: entry numbers will likely shift after compile.",
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("Run(read 1) stderr = %q, want %q", stderr.String(), want)
		}
	}
}

func TestReadNumericReferenceWarnsWhenManifestHashDiffersFromBrief(t *testing.T) {
	root := makeCLIVault(t)
	writeCLIFile(t, root, "note.md", "# Note\n\nSummary.\n")

	var compileStdout, compileStderr bytes.Buffer
	code := Run([]string{"compile", "--dir", root}, &compileStdout, &compileStderr)
	if code != 0 {
		t.Fatalf("Run(compile) exit code = %d, want 0; stderr = %q", code, compileStderr.String())
	}
	manifestPath := filepath.Join(root, ".memento", "manifest.json")
	manifestJSON := readCLIFile(t, root, ".memento/manifest.json")
	manifestJSON = strings.Replace(manifestJSON, `"title": "Note"`, `"title": "Changed"`, 1)
	if err := os.WriteFile(manifestPath, []byte(manifestJSON), 0o644); err != nil {
		t.Fatalf("write changed manifest: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code = Run([]string{"read", "--dir", root, "1"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(read 1) exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if want := "# Note\n\nSummary.\n"; stdout.String() != want {
		t.Fatalf("Run(read 1) stdout = %q, want %q", stdout.String(), want)
	}
	if want := "warn: manifest changed since last brief, numbers may not match your view"; !strings.Contains(stderr.String(), want) {
		t.Fatalf("Run(read 1) stderr = %q, want %q", stderr.String(), want)
	}
}

func TestReadNumericReferenceSkipsHashCheckWhenBriefIsMissing(t *testing.T) {
	root := makeCLIVault(t)
	writeCLIFile(t, root, "note.md", "# Note\n\nSummary.\n")

	var compileStdout, compileStderr bytes.Buffer
	code := Run([]string{"compile", "--dir", root}, &compileStdout, &compileStderr)
	if code != 0 {
		t.Fatalf("Run(compile) exit code = %d, want 0; stderr = %q", code, compileStderr.String())
	}
	if err := os.Remove(filepath.Join(root, "_memento", "brief.md")); err != nil {
		t.Fatalf("remove brief: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code = Run([]string{"read", "--dir", root, "1"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(read 1) exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("Run(read 1) stderr = %q, want empty", stderr.String())
	}
	if want := "# Note\n\nSummary.\n"; stdout.String() != want {
		t.Fatalf("Run(read 1) stdout = %q, want %q", stdout.String(), want)
	}
}

func TestReadNonNumericArgumentFallsThroughToPathBehavior(t *testing.T) {
	root := makeCLIVault(t)
	writeCLIFile(t, root, "2026.md", "# Year\n\nPath note.\n")

	var stdout, stderr bytes.Buffer
	code := Run([]string{"read", "--dir", root, "2026.md"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(read 2026.md) exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("Run(read 2026.md) stderr = %q, want empty", stderr.String())
	}
	if want := "# Year\n\nPath note.\n"; stdout.String() != want {
		t.Fatalf("Run(read 2026.md) stdout = %q, want %q", stdout.String(), want)
	}
}

func TestReadPrintsRequestedSection(t *testing.T) {
	root := makeCLIVault(t)
	writeCLIFile(t, root, "spec.md", "# Spec\n\n## Target Heading\n\nTarget content.\n\n## Next\n\nOther content.\n")

	var stdout, stderr bytes.Buffer
	code := Run([]string{"read", "--dir", root, "spec.md#target-heading"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(read section) exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("Run(read section) stderr = %q, want empty", stderr.String())
	}

	want := "## Target Heading\n\nTarget content.\n\n"
	if stdout.String() != want {
		t.Fatalf("Run(read section) stdout = %q, want %q", stdout.String(), want)
	}
}

func TestReadFailsClearlyForUnknownSection(t *testing.T) {
	root := makeCLIVault(t)
	writeCLIFile(t, root, "spec.md", "# Spec\n\n## Present\n\nContent.\n")

	var stdout, stderr bytes.Buffer
	code := Run([]string{"read", "--dir", root, "spec.md#missing"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("Run(read unknown section) exit code = %d, want 1", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("Run(read unknown section) stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "section not found") {
		t.Fatalf("Run(read unknown section) stderr = %q, want section not found message", stderr.String())
	}
}

func TestReadFailsClearlyForMissingOrIgnoredKey(t *testing.T) {
	root := makeCLIVault(t)
	writeCLIFile(t, root, ".mementoignore", "ignored.md\n")
	writeCLIFile(t, root, "ignored.md", "# Ignored\n")

	for _, key := range []string{"missing.md", "ignored.md"} {
		t.Run(key, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := Run([]string{"read", "--dir", root, key}, &stdout, &stderr)
			if code != 1 {
				t.Fatalf("Run(read %s) exit code = %d, want 1", key, code)
			}
			if stdout.Len() != 0 {
				t.Fatalf("Run(read %s) stdout = %q, want empty", key, stdout.String())
			}
			if !strings.Contains(stderr.String(), "not found") {
				t.Fatalf("Run(read %s) stderr = %q, want not found message", key, stderr.String())
			}
		})
	}
}

func TestReadRejectsTraversalKey(t *testing.T) {
	root := makeCLIVault(t)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"read", "--dir", root, "../outside.md"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("Run(read traversal) exit code = %d, want 1", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("Run(read traversal) stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "invalid key") {
		t.Fatalf("Run(read traversal) stderr = %q, want invalid key message", stderr.String())
	}
}

func TestWriteCreatesMarkdownFromStdin(t *testing.T) {
	root := makeCLIVault(t)

	var stdout, stderr bytes.Buffer
	code := RunWithInput(
		[]string{"write", "--dir", root, "notes/new.md"},
		strings.NewReader("# New\n\nDurable note.\n"),
		&stdout,
		&stderr,
	)
	if code != 0 {
		t.Fatalf("Run(write create) exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("Run(write create) stdout = %q, want empty", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("Run(write create) stderr = %q, want empty", stderr.String())
	}

	want := "# New\n\nDurable note.\n"
	if got := readCLIFile(t, root, "notes/new.md"); got != want {
		t.Fatalf("written note = %q, want %q", got, want)
	}
}

func TestWriteAppendsMarkdownFromStdin(t *testing.T) {
	root := makeCLIVault(t)
	writeCLIFile(t, root, "note.md", "# Note\n\nExisting.\n")

	var stdout, stderr bytes.Buffer
	code := RunWithInput(
		[]string{"write", "--dir", root, "note.md"},
		strings.NewReader("\nAppended.\n"),
		&stdout,
		&stderr,
	)
	if code != 0 {
		t.Fatalf("Run(write append) exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("Run(write append) stdout = %q, want empty", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("Run(write append) stderr = %q, want empty", stderr.String())
	}

	want := "# Note\n\nExisting.\n\nAppended.\n"
	if got := readCLIFile(t, root, "note.md"); got != want {
		t.Fatalf("appended note = %q, want %q", got, want)
	}
}

func TestWriteRejectsTraversalKey(t *testing.T) {
	root := makeCLIVault(t)

	var stdout, stderr bytes.Buffer
	code := RunWithInput(
		[]string{"write", "--dir", root, "../outside.md"},
		strings.NewReader("# Outside\n"),
		&stdout,
		&stderr,
	)
	if code != 1 {
		t.Fatalf("Run(write traversal) exit code = %d, want 1", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("Run(write traversal) stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "invalid key") {
		t.Fatalf("Run(write traversal) stderr = %q, want invalid key message", stderr.String())
	}
}

func TestWriteDoesNotOfferDeferredMutationFlags(t *testing.T) {
	root := makeCLIVault(t)
	writeCLIFile(t, root, "note.md", "# Note\n\nOriginal.\n")

	for _, args := range [][]string{
		{"write", "--dir", root, "--overwrite", "note.md"},
		{"write", "--dir", root, "--section", "context", "note.md"},
		{"write", "--dir", root, "--upsert", "key", "note.md"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := RunWithInput(args, strings.NewReader("replacement\n"), &stdout, &stderr)
			if code != 2 {
				t.Fatalf("Run(%v) exit code = %d, want 2", args, code)
			}
			if stdout.Len() != 0 {
				t.Fatalf("Run(%v) stdout = %q, want empty", args, stdout.String())
			}
			if got := readCLIFile(t, root, "note.md"); got != "# Note\n\nOriginal.\n" {
				t.Fatalf("note changed after unsupported mutation flag: %q", got)
			}
		})
	}
}

func makeCLIVault(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".memento"), 0o755); err != nil {
		t.Fatalf("mkdir marker: %v", err)
	}
	return root
}

func writeCLIFile(t *testing.T, root, relPath, content string) {
	t.Helper()

	path := filepath.Join(root, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir parent for %q: %v", relPath, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %q: %v", relPath, err)
	}
}

func readCLIFile(t *testing.T, root, relPath string) string {
	t.Helper()

	path := filepath.Join(root, filepath.FromSlash(relPath))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %q: %v", relPath, err)
	}
	return string(data)
}

func chdirCLI(t *testing.T, dir string) {
	t.Helper()

	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %q: %v", dir, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previous); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})
}
