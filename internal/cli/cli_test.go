package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tpisel/memento/internal/brief"
	"github.com/tpisel/memento/internal/manifest"
	"github.com/tpisel/memento/internal/note"
	"github.com/tpisel/memento/internal/orient"
	"github.com/tpisel/memento/internal/vault"
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
		"orient",
		"read",
		"memento read <key|@N>",
		"version",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("Run(help) output %q does not contain %q", out, want)
		}
	}
}

func TestSubcommandHelp(t *testing.T) {
	tests := []struct {
		verb string
		want []string
	}{
		{
			verb: "brief",
			want: []string{
				"Usage:",
				"memento brief",
				"Print the agent-facing manifest projection",
				"No flags.",
				"memento orient",
			},
		},
		{
			verb: "compile",
			want: []string{
				"Usage:",
				"memento compile",
				"Rebuild .memento/manifest.json and _memento/brief.md",
				"No flags.",
				"memento orient",
			},
		},
		{
			verb: "init",
			want: []string{
				"Usage:",
				"memento init [--dir <vault>]",
				"Adopt or create a memory vault",
				"--dir <vault>",
				"memento orient",
			},
		},
		{
			verb: "orient",
			want: []string{
				"Usage:",
				"memento orient",
				"Print tool-usage orientation",
				"No flags.",
				"memento orient",
			},
		},
		{
			verb: "read",
			want: []string{
				"Usage:",
				"memento read <key|@N>",
				"memento read <key|@N>#<heading>",
				"vault-relative .md path",
				"@N reads the numbered entry",
				"key#heading and @N#heading",
				"stderr begins with binding: ratified|unratified",
				"summary: current|stale|missing",
				"inlinks:, outlinks:, transcludes:, transcluded-by:",
				"memento orient",
			},
		},
		{
			verb: "write",
			want: []string{
				"Usage:",
				"memento write [--overwrite] <key>",
				"vault-relative .md path",
				"--overwrite",
				"write appends by default",
				"append-only is the default",
				"living accepts appends and overwrites",
				"read-only rejects writes after ratification",
				"edit window",
				"wrote: <abs path> (<byte count>, <append|overwrite>)",
				"memento orient",
			},
		},
	}

	for _, tt := range tests {
		for _, helpFlag := range []string{"-h", "--help"} {
			t.Run(tt.verb+" "+helpFlag, func(t *testing.T) {
				var stdout, stderr bytes.Buffer

				code := Run([]string{tt.verb, helpFlag}, &stdout, &stderr)
				if code != 0 {
					t.Fatalf("Run(%s %s) exit code = %d, want 0; stderr = %q", tt.verb, helpFlag, code, stderr.String())
				}
				if stderr.Len() != 0 {
					t.Fatalf("Run(%s %s) stderr = %q, want empty", tt.verb, helpFlag, stderr.String())
				}
				out := stdout.String()
				if out == "" {
					t.Fatalf("Run(%s %s) stdout = empty, want help", tt.verb, helpFlag)
				}
				for _, want := range tt.want {
					if !strings.Contains(out, want) {
						t.Fatalf("Run(%s %s) stdout =\n%s\nwant substring %q", tt.verb, helpFlag, out, want)
					}
				}
			})
		}
	}
}

func TestReadmeCurrentVerbsMatchHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := Run([]string{"help"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(help) exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("Run(help) wrote stderr = %q, want empty", stderr.String())
	}

	readme := readRepoFile(t, "README.md")
	got := readmeCurrentVerbUsages(readme)
	want := helpUsageLines(stdout.String())
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("README current verbs = %v, want help usage = %v", got, want)
	}

	gotNames := commandNamesFromUsages(got)
	wantNames := helpCommandNames(stdout.String())
	if strings.Join(gotNames, "\n") != strings.Join(wantNames, "\n") {
		t.Fatalf("README current verb names = %v, want help commands = %v", gotNames, wantNames)
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
	assertRootErrorToken(t, stderr.String(), "unknown-command")
	if !strings.Contains(stderr.String(), `unknown command "bogus"`) {
		t.Fatalf("Run(bogus) stderr = %q, want unknown command message", stderr.String())
	}
}

func TestRemovedNonInitFlagsFailWithInvalidArguments(t *testing.T) {
	root := t.TempDir()

	tests := []struct {
		name  string
		verb  string
		args  []string
		input io.Reader
	}{
		{name: "brief dir", verb: "brief", args: []string{"brief", "--dir", root}},
		{name: "compile dir", verb: "compile", args: []string{"compile", "--dir", root}},
		{name: "compile print", verb: "compile", args: []string{"compile", "--print"}},
		{name: "orient dir", verb: "orient", args: []string{"orient", "--dir", root}},
		{name: "read dir", verb: "read", args: []string{"read", "--dir", root, "note.md"}},
		{name: "write dir", verb: "write", args: []string{"write", "--dir", root, "note.md"}, input: strings.NewReader("body\n")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			var code int
			if tt.input != nil {
				code = RunWithInput(tt.args, tt.input, &stdout, &stderr)
			} else {
				code = Run(tt.args, &stdout, &stderr)
			}
			if code != 2 {
				t.Fatalf("Run(%v) exit code = %d, want 2; stderr = %q", tt.args, code, stderr.String())
			}
			if stdout.Len() != 0 {
				t.Fatalf("Run(%v) stdout = %q, want empty", tt.args, stdout.String())
			}
			assertCLIErrorToken(t, stderr.String(), tt.verb, "invalid-arguments")
		})
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
	if got, want := stderr.String(), compiledStatusLine(1); got != want {
		t.Fatalf("Run(compile) stderr = %q, want %q", got, want)
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
		"---\nmode: read-only\nmanifest: sha256:",
		"mode: read-only",
		"> [!caution] Auto-generated by `memento compile`",
		"> Any edits to this file will be overwritten on the next compile run.",
		"## (root)",
		"### @1. Note",
		"key: `note.md` | mode: `append-only`",
		"Section read: memento read <key|@N>#<heading>",
	} {
		if !strings.Contains(string(brief), want) {
			t.Fatalf("brief contents = %q, want %q", string(brief), want)
		}
	}
}

func TestNonInitVerbsDoNotRefreshAgentInstructionsBootloader(t *testing.T) {
	repo := t.TempDir()
	root := filepath.Join(repo, "project-memory")
	if err := os.MkdirAll(filepath.Join(root, ".memento"), 0o755); err != nil {
		t.Fatalf("mkdir marker: %v", err)
	}
	writeCLIFile(t, root, "note.md", "# Note\n\nSummary.\n")
	agentInstructions := "# Agent Instructions\n\n<!-- memento:start -->\nold bootloader block\n<!-- memento:end -->\n"
	writeCLIFile(t, repo, "AGENTS.md", agentInstructions)
	chdirCLI(t, repo)

	for _, args := range [][]string{
		{"compile"},
		{"brief"},
		{"orient"},
		{"read", "note.md"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := Run(args, &stdout, &stderr)
			if code != 0 {
				t.Fatalf("Run(%v) exit code = %d, want 0; stderr = %q", args, code, stderr.String())
			}
			if got := readCLIFile(t, repo, "AGENTS.md"); got != agentInstructions {
				t.Fatalf("AGENTS.md changed after Run(%v):\ngot:\n%s\nwant:\n%s", args, got, agentInstructions)
			}
		})
	}
}

func TestCompileWritesManifestAndWarnsWhenBriefWriteFails(t *testing.T) {
	root := makeCLIVault(t)
	writeCLIFile(t, root, "note.md", "# Note\n\nSummary.\n")
	writeCLIFile(t, root, "_memento", "not a directory\n")

	var stdout, stderr bytes.Buffer
	code := Run([]string{"compile"}, &stdout, &stderr)
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
	code := Run([]string{"compile"}, &stdout, &stderr)
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

func TestCompileBriefSuffixesWikiLinksWithoutMutatingSources(t *testing.T) {
	root := makeCLIVault(t)
	source := `# Alpha

Resolved [[beta]], display [[beta|Beta note]], broken [[missing]], and anchored [[beta#Decision]] links.
`
	writeCLIFile(t, root, "alpha.md", source)
	writeCLIFile(t, root, "beta.md", "# Beta\n\nTarget.\n")

	var stdout, stderr bytes.Buffer
	code := Run([]string{"compile"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(compile) exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("Run(compile) stdout = %q, want empty", stdout.String())
	}
	if got, want := stderr.String(), compiledStatusLine(2); got != want {
		t.Fatalf("Run(compile) stderr = %q, want %q", got, want)
	}

	brief := readCLIFile(t, root, "_memento/brief.md")
	want := "Resolved [[beta|beta @2]], display [[beta|Beta note @2]], broken [[missing]], and anchored [[beta#Decision]] links."
	if !strings.Contains(brief, want) {
		t.Fatalf("brief =\n%s\nwant %q", brief, want)
	}
	if got := readCLIFile(t, root, "alpha.md"); got != source {
		t.Fatalf("source mutated by compile:\ngot:\n%s\nwant:\n%s", got, source)
	}
}

func TestBriefPrintsExistingBriefForExplicitDir(t *testing.T) {
	root := makeCLIVault(t)
	want := "# Existing Brief\n\nAlready rendered.\n"
	writeCLIFile(t, root, "_memento/brief.md", want)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"brief"}, &stdout, &stderr)
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
	code := Run([]string{"compile"}, &compileStdout, &compileStderr)
	if code != 0 {
		t.Fatalf("Run(compile) exit code = %d, want 0; stderr = %q", code, compileStderr.String())
	}
	briefPath := filepath.Join(root, "_memento", "brief.md")
	if err := os.Remove(briefPath); err != nil {
		t.Fatalf("remove brief: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code = Run([]string{"brief"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(brief --dir) exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("Run(brief --dir) stderr = %q, want empty", stderr.String())
	}
	for _, want := range []string{
		"---\nmode: read-only\nmanifest: sha256:",
		"mode: read-only",
		"> [!caution] Auto-generated by `memento compile`",
		"> Any edits to this file will be overwritten on the next compile run.",
		"## (root)",
		"### @1. Note",
		"key: `note.md` | mode: `append-only`",
		"Section read: memento read <key|@N>#<heading>",
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
	makeCLIVault(t)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"brief"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("Run(brief --dir) exit code = %d, want 1", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("Run(brief --dir) stdout = %q, want empty", stdout.String())
	}
	assertCLIErrorToken(t, stderr.String(), "brief", "manifest-not-found")
	for _, want := range []string{"manifest", "run: memento compile"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("Run(brief --dir) stderr = %q, want %q", stderr.String(), want)
		}
	}
}

func TestOrientPrintsBaselineOnlyWhenNoDocsAreTagged(t *testing.T) {
	root := makeCLIVault(t)
	writeCLIFile(t, root, "note.md", "# Note\n\nSummary.\n")

	var compileStdout, compileStderr bytes.Buffer
	code := Run([]string{"compile"}, &compileStdout, &compileStderr)
	if code != 0 {
		t.Fatalf("Run(compile) exit code = %d, want 0; stderr = %q", code, compileStderr.String())
	}

	var stdout, stderr bytes.Buffer
	code = Run([]string{"orient"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(orient) exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("Run(orient) stderr = %q, want empty", stderr.String())
	}
	if got, want := stdout.String(), renderedBaselineWithoutWritingGuide(t, root); got != want {
		t.Fatalf("Run(orient) stdout =\n%s\nwant baseline:\n%s", got, want)
	}
}

func TestOrientIncludesWritingGuidePreconditionWhenWritingGuideExists(t *testing.T) {
	root := makeCLIVault(t)
	writeCLIFile(t, root, "note.md", "# Note\n\nSummary.\n")
	writeCLIFile(t, root, "_memento/writing.md", "---\nmode: read-only\n---\n# Writing\n\nUse judgement.\n")

	var compileStdout, compileStderr bytes.Buffer
	code := Run([]string{"compile"}, &compileStdout, &compileStderr)
	if code != 0 {
		t.Fatalf("Run(compile) exit code = %d, want 0; stderr = %q", code, compileStderr.String())
	}

	var stdout, stderr bytes.Buffer
	code = Run([]string{"orient"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(orient) exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("Run(orient) stderr = %q, want empty", stderr.String())
	}

	want := "before authoring, run `memento read _memento/writing.md`."
	if !strings.Contains(stdout.String(), want) {
		t.Fatalf("Run(orient) stdout =\n%s\nwant writing guide precondition %q", stdout.String(), want)
	}
}

func TestOrientOmitsWritingGuidePreconditionWhenWritingGuideIsAbsent(t *testing.T) {
	root := makeCLIVault(t)
	writeCLIFile(t, root, "note.md", "# Note\n\nSummary.\n")

	var compileStdout, compileStderr bytes.Buffer
	code := Run([]string{"compile"}, &compileStdout, &compileStderr)
	if code != 0 {
		t.Fatalf("Run(compile) exit code = %d, want 0; stderr = %q", code, compileStderr.String())
	}

	var stdout, stderr bytes.Buffer
	code = Run([]string{"orient"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(orient) exit code = %d, want 0; stderr = %q", code, stderr.String())
	}

	out := stdout.String()
	for _, unwanted := range []string{
		"before authoring, run `memento read _memento/writing.md`.",
		"consider writing one",
	} {
		if strings.Contains(out, unwanted) {
			t.Fatalf("Run(orient) stdout =\n%s\nwant no writing guide text containing %q", out, unwanted)
		}
	}
}

func TestOrientAppendsSingleTaggedDocAfterBaseline(t *testing.T) {
	root := makeCLIVault(t)
	overlay := "---\norient: true\n---\n# Project Orientation\n\nUse the project guide.\n"
	writeCLIFile(t, root, "orientation.md", overlay)
	writeCLIFile(t, root, "note.md", "# Note\n\nSummary.\n")

	var compileStdout, compileStderr bytes.Buffer
	code := Run([]string{"compile"}, &compileStdout, &compileStderr)
	if code != 0 {
		t.Fatalf("Run(compile) exit code = %d, want 0; stderr = %q", code, compileStderr.String())
	}

	var stdout, stderr bytes.Buffer
	code = Run([]string{"orient"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(orient) exit code = %d, want 0; stderr = %q", code, stderr.String())
	}

	want := strings.TrimRight(renderedBaselineWithoutWritingGuide(t, root), "\n") + "\n---\n\n" + strings.TrimRight(overlay, "\n") + "\n"
	if stdout.String() != want {
		t.Fatalf("Run(orient) stdout =\n%s\nwant:\n%s", stdout.String(), want)
	}
}

func renderedBaselineWithoutWritingGuide(t *testing.T, root string) string {
	t.Helper()
	v := vault.Vault{
		Root:         root,
		MarkerDir:    filepath.Join(root, vault.MarkerDirName),
		ManifestPath: filepath.Join(root, vault.MarkerDirName, vault.ManifestFileName),
	}
	m, err := manifest.Load(v)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	out := strings.Replace(string(orient.Baseline()), "<!-- memento:triggered-preconditions -->", "None yet.", 1)
	disclosure := "`memento brief` will report no notes yet."
	if len(m.Entries) > 0 {
		lines := bytes.Count(brief.Render(m), []byte("\n"))
		noun := "notes"
		if len(m.Entries) == 1 {
			noun = "note"
		}
		disclosure = fmt.Sprintf("`memento brief` will print summaries of %d %s (~%d lines); it is dense and pull-only.", len(m.Entries), noun, lines)
	}
	return strings.Replace(out, "<!-- memento:brief-disclosure -->", disclosure, 1)
}

func TestOrientSortsMultipleTaggedDocsByManifestKey(t *testing.T) {
	root := makeCLIVault(t)
	writeCLIFile(t, root, "zeta.md", "---\norient: true\n---\n# Zeta\n")
	writeCLIFile(t, root, "alpha.md", "---\norient: true\n---\n# Alpha\n")
	writeCLIFile(t, root, "nested/beta.md", "---\norient: true\n---\n# Beta\n")

	var compileStdout, compileStderr bytes.Buffer
	code := Run([]string{"compile"}, &compileStdout, &compileStderr)
	if code != 0 {
		t.Fatalf("Run(compile) exit code = %d, want 0; stderr = %q", code, compileStderr.String())
	}

	var stdout, stderr bytes.Buffer
	code = Run([]string{"orient"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(orient) exit code = %d, want 0; stderr = %q", code, stderr.String())
	}

	out := stdout.String()
	alpha := strings.Index(out, "# Alpha")
	beta := strings.Index(out, "# Beta")
	zeta := strings.Index(out, "# Zeta")
	if alpha < 0 || beta < 0 || zeta < 0 {
		t.Fatalf("Run(orient) stdout =\n%s\nwant all tagged docs", out)
	}
	if !(alpha < beta && beta < zeta) {
		t.Fatalf("Run(orient) overlay order indexes alpha=%d beta=%d zeta=%d; want key order", alpha, beta, zeta)
	}
}

func TestOrientExcludesUntaggedAndExplicitFalseDocs(t *testing.T) {
	root := makeCLIVault(t)
	writeCLIFile(t, root, "include.md", "---\norient: true\n---\n# Include\n")
	writeCLIFile(t, root, "untagged.md", "# Untagged\n")
	writeCLIFile(t, root, "false.md", "---\norient: false\n---\n# False\n")

	var compileStdout, compileStderr bytes.Buffer
	code := Run([]string{"compile"}, &compileStdout, &compileStderr)
	if code != 0 {
		t.Fatalf("Run(compile) exit code = %d, want 0; stderr = %q", code, compileStderr.String())
	}

	var stdout, stderr bytes.Buffer
	code = Run([]string{"orient"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(orient) exit code = %d, want 0; stderr = %q", code, stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, "# Include") {
		t.Fatalf("Run(orient) stdout =\n%s\nwant included tagged doc", out)
	}
	for _, forbidden := range []string{"# Untagged", "# False"} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("Run(orient) stdout contains excluded doc %q:\n%s", forbidden, out)
		}
	}
}

func TestOrientFailsWithCompileHintWhenManifestIsMissing(t *testing.T) {
	makeCLIVault(t)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"orient"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("Run(orient --dir) exit code = %d, want 1", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("Run(orient --dir) stdout = %q, want empty", stdout.String())
	}
	assertCLIErrorToken(t, stderr.String(), "orient", "manifest-not-found")
	for _, want := range []string{"manifest", "run: memento compile"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("Run(orient --dir) stderr = %q, want %q", stderr.String(), want)
		}
	}
}

func TestReadWithoutManifestPrintsRequestedMarkdownAndLinkSurfaceHint(t *testing.T) {
	root := makeCLIVault(t)
	writeCLIFile(t, root, "notes/deep.md", "# Deep\n\nNested content.\n")

	var stdout, stderr bytes.Buffer
	code := Run([]string{"read", "notes/deep.md"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(read --dir notes/deep.md) exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	wantStderr := "binding: ratified\nnote: no manifest; link surface unavailable. run: memento compile\n"
	if got := stderr.String(); got != wantStderr {
		t.Fatalf("Run(read) stderr = %q, want %q", got, wantStderr)
	}
	for _, forbidden := range []string{"inlinks:", "outlinks:", "transcludes:", "transcluded-by:"} {
		if strings.Contains(stderr.String(), forbidden) {
			t.Fatalf("Run(read) stderr = %q, want no role line %q", stderr.String(), forbidden)
		}
	}

	want := "# Deep\n\nNested content.\n"
	if stdout.String() != want {
		t.Fatalf("Run(read) stdout = %q, want %q", stdout.String(), want)
	}
}

func TestReadPrintsUnratifiedBindingForUntrackedGitNote(t *testing.T) {
	root := makeCLIVault(t)
	initCLIGit(t, root)
	writeCLIFile(t, root, "note.md", "# Note\n\nDraft.\n")

	var stdout, stderr bytes.Buffer
	code := Run([]string{"read", "note.md"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(read unratified) exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	wantStderr := "binding: unratified\nnote: no manifest; link surface unavailable. run: memento compile\n"
	if got := stderr.String(); got != wantStderr {
		t.Fatalf("Run(read unratified) stderr = %q, want %q", got, wantStderr)
	}
	if want := "# Note\n\nDraft.\n"; stdout.String() != want {
		t.Fatalf("Run(read unratified) stdout = %q, want %q", stdout.String(), want)
	}
}

func TestReadNumericReferenceResolvesAgainstManifestOrdering(t *testing.T) {
	root := makeCLIVault(t)
	writeCLIFile(t, root, "zeta.md", "# Zeta\n\nRoot note.\n")
	writeCLIFile(t, root, "notes/beta.md", "# Beta\n\nNested note.\n")

	var compileStdout, compileStderr bytes.Buffer
	code := Run([]string{"compile"}, &compileStdout, &compileStderr)
	if code != 0 {
		t.Fatalf("Run(compile) exit code = %d, want 0; stderr = %q", code, compileStderr.String())
	}

	var stdout, stderr bytes.Buffer
	code = Run([]string{"read", "@2"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(read @2) exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if got, want := stderr.String(), "binding: ratified\nsummary: missing\n"; got != want {
		t.Fatalf("Run(read @2) stderr = %q, want %q", got, want)
	}
	if want := "# Beta\n\nNested note.\n"; stdout.String() != want {
		t.Fatalf("Run(read @2) stdout = %q, want %q", stdout.String(), want)
	}
}

func TestReadNumericReferenceSupportsSectionRead(t *testing.T) {
	root := makeCLIVault(t)
	writeCLIFile(t, root, "alpha.md", "# Alpha\n\n## Target Heading\n\nTarget content.\n\n## Next\n\nOther content.\n")
	writeCLIFile(t, root, "beta.md", "# Beta\n\nOther note.\n")

	var compileStdout, compileStderr bytes.Buffer
	code := Run([]string{"compile"}, &compileStdout, &compileStderr)
	if code != 0 {
		t.Fatalf("Run(compile) exit code = %d, want 0; stderr = %q", code, compileStderr.String())
	}

	var numericStdout, numericStderr bytes.Buffer
	code = Run([]string{"read", "@1#target-heading"}, &numericStdout, &numericStderr)
	if code != 0 {
		t.Fatalf("Run(read @1#target-heading) exit code = %d, want 0; stderr = %q", code, numericStderr.String())
	}

	var keyStdout, keyStderr bytes.Buffer
	code = Run([]string{"read", "alpha.md#target-heading"}, &keyStdout, &keyStderr)
	if code != 0 {
		t.Fatalf("Run(read alpha.md#target-heading) exit code = %d, want 0; stderr = %q", code, keyStderr.String())
	}

	if got, want := numericStdout.String(), keyStdout.String(); got != want {
		t.Fatalf("Run(read @1#target-heading) stdout = %q, want key section stdout %q", got, want)
	}
	if got, want := numericStdout.String(), "## Target Heading\n\nTarget content.\n\n"; got != want {
		t.Fatalf("Run(read @1#target-heading) stdout = %q, want %q", got, want)
	}
	if got, want := numericStderr.String(), keyStderr.String(); got != want {
		t.Fatalf("Run(read @1#target-heading) stderr = %q, want key section stderr %q", got, want)
	}
}

func TestReadEmitsSummaryStateFromManifest(t *testing.T) {
	root := makeCLIVault(t)
	writeCLIFile(t, root, "note.md", "---\nsummary: Committed summary.\n---\n# Note\n\nBody.\n")

	var compileStdout, compileStderr bytes.Buffer
	code := Run([]string{"compile"}, &compileStdout, &compileStderr)
	if code != 0 {
		t.Fatalf("Run(compile) exit code = %d, want 0; stderr = %q", code, compileStderr.String())
	}

	var stdout, stderr bytes.Buffer
	code = Run([]string{"read", "note.md"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(read note.md) exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if got, want := stderr.String(), "binding: ratified\nsummary: current\n"; got != want {
		t.Fatalf("Run(read note.md) stderr = %q, want %q", got, want)
	}
	if want := "---\nsummary: Committed summary.\n---\n# Note\n\nBody.\n"; stdout.String() != want {
		t.Fatalf("Run(read note.md) stdout = %q, want %q", stdout.String(), want)
	}
}

func TestReadEmitsLinkSurfaceOnStderrWithoutChangingStdout(t *testing.T) {
	root := makeCLIVault(t)
	body := "# Subject\n\nLinks to [[outbound.md]], [[Missing Target#Anchor]], and ![[embed-out.md]].\n"
	writeCLIFile(t, root, "subject.md", body)
	writeCLIFile(t, root, "aaa-in.md", "# In\n\nPoints to [[subject.md]].\n")
	writeCLIFile(t, root, "embed-in.md", "# Embed In\n\nQuotes ![[subject.md]].\n")
	writeCLIFile(t, root, "outbound.md", "# Outbound\n")
	writeCLIFile(t, root, "embed-out.md", "# Embed Out\n")

	var compileStdout, compileStderr bytes.Buffer
	code := Run([]string{"compile"}, &compileStdout, &compileStderr)
	if code != 0 {
		t.Fatalf("Run(compile) exit code = %d, want 0; stderr = %q", code, compileStderr.String())
	}

	var stdout, stderr bytes.Buffer
	code = Run([]string{"read", "subject.md"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(read subject.md) exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if stdout.String() != body {
		t.Fatalf("Run(read subject.md) stdout = %q, want source body %q", stdout.String(), body)
	}

	wantStderr := strings.Join([]string{
		"binding: ratified",
		"summary: missing",
		"inlinks: aaa-in.md @1",
		"outlinks: Missing Target#Anchor, outbound.md @4",
		"transcludes: embed-out.md @3",
		"transcluded-by: embed-in.md @2",
		"",
	}, "\n")
	if got := stderr.String(); got != wantStderr {
		t.Fatalf("Run(read subject.md) stderr = %q, want %q", got, wantStderr)
	}
}

func TestReadOmitsEmptyLinkRoles(t *testing.T) {
	root := makeCLIVault(t)
	writeCLIFile(t, root, "subject.md", "# Subject\n\nOnly [[target.md]].\n")
	writeCLIFile(t, root, "target.md", "# Target\n")

	var compileStdout, compileStderr bytes.Buffer
	code := Run([]string{"compile"}, &compileStdout, &compileStderr)
	if code != 0 {
		t.Fatalf("Run(compile) exit code = %d, want 0; stderr = %q", code, compileStderr.String())
	}

	var stdout, stderr bytes.Buffer
	code = Run([]string{"read", "subject.md"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(read subject.md) exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if got, want := stderr.String(), "binding: ratified\nsummary: missing\noutlinks: target.md @2\n"; got != want {
		t.Fatalf("Run(read subject.md) stderr = %q, want %q", got, want)
	}
	for _, forbidden := range []string{"inlinks:", "transcludes:", "transcluded-by:", "none"} {
		if strings.Contains(stderr.String(), forbidden) {
			t.Fatalf("Run(read subject.md) stderr = %q, want no %q", stderr.String(), forbidden)
		}
	}
}

func TestReadEmptyLinkDocEmitsOnlyBinding(t *testing.T) {
	root := makeCLIVault(t)
	writeCLIFile(t, root, "subject.md", "# Subject\n\nNo links.\n")

	var compileStdout, compileStderr bytes.Buffer
	code := Run([]string{"compile"}, &compileStdout, &compileStderr)
	if code != 0 {
		t.Fatalf("Run(compile) exit code = %d, want 0; stderr = %q", code, compileStderr.String())
	}

	var stdout, stderr bytes.Buffer
	code = Run([]string{"read", "subject.md"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(read subject.md) exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if got, want := stderr.String(), "binding: ratified\nsummary: missing\n"; got != want {
		t.Fatalf("Run(read subject.md) stderr = %q, want %q", got, want)
	}
}

func TestReadSkipsBareSameDocAnchorWikiLinks(t *testing.T) {
	root := makeCLIVault(t)
	writeCLIFile(t, root, "subject.md", "# Subject\n\nSee [[#local-heading]].\n\n## Local Heading\n\nDetails.\n")

	var compileStdout, compileStderr bytes.Buffer
	code := Run([]string{"compile"}, &compileStdout, &compileStderr)
	if code != 0 {
		t.Fatalf("Run(compile) exit code = %d, want 0; stderr = %q", code, compileStderr.String())
	}

	var stdout, stderr bytes.Buffer
	code = Run([]string{"read", "subject.md"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(read subject.md) exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if got, want := stderr.String(), "binding: ratified\nsummary: missing\n"; got != want {
		t.Fatalf("Run(read subject.md) stderr = %q, want %q", got, want)
	}
	if strings.Contains(stderr.String(), "outlinks:") {
		t.Fatalf("Run(read subject.md) stderr = %q, want no outlinks line", stderr.String())
	}
}

func TestReadNumericReferenceEmitsResolvedEntryLinkSurface(t *testing.T) {
	root := makeCLIVault(t)
	writeCLIFile(t, root, "alpha.md", "# Alpha\n\nSee [[beta.md]].\n")
	writeCLIFile(t, root, "beta.md", "# Beta\n")

	var compileStdout, compileStderr bytes.Buffer
	code := Run([]string{"compile"}, &compileStdout, &compileStderr)
	if code != 0 {
		t.Fatalf("Run(compile) exit code = %d, want 0; stderr = %q", code, compileStderr.String())
	}

	var stdout, stderr bytes.Buffer
	code = Run([]string{"read", "@1"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(read @1) exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if got, want := stderr.String(), "binding: ratified\nsummary: missing\noutlinks: beta.md @2\n"; got != want {
		t.Fatalf("Run(read @1) stderr = %q, want %q", got, want)
	}
}

func TestReadSeparatesWikiLinksAndEmbeds(t *testing.T) {
	root := makeCLIVault(t)
	writeCLIFile(t, root, "subject.md", "# Subject\n\nPlain [[plain.md]] and embedded ![[embedded.md]].\n")
	writeCLIFile(t, root, "plain.md", "# Plain\n")
	writeCLIFile(t, root, "embedded.md", "# Embedded\n")

	var compileStdout, compileStderr bytes.Buffer
	code := Run([]string{"compile"}, &compileStdout, &compileStderr)
	if code != 0 {
		t.Fatalf("Run(compile) exit code = %d, want 0; stderr = %q", code, compileStderr.String())
	}

	var stdout, stderr bytes.Buffer
	code = Run([]string{"read", "subject.md"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(read subject.md) exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if got, want := stderr.String(), "binding: ratified\nsummary: missing\noutlinks: plain.md @2\ntranscludes: embedded.md @1\n"; got != want {
		t.Fatalf("Run(read subject.md) stderr = %q, want %q", got, want)
	}
}

func TestReadSectionScopesOutlinksToExcerpt(t *testing.T) {
	root := makeCLIVault(t)
	writeCLIFile(t, root, "subject.md", "# Subject\n\n## A\n\nA links to [[a-target.md]].\n\n## B\n\nB links to [[b-target.md]].\n")
	writeCLIFile(t, root, "a-target.md", "# A Target\n")
	writeCLIFile(t, root, "b-target.md", "# B Target\n")

	var compileStdout, compileStderr bytes.Buffer
	code := Run([]string{"compile"}, &compileStdout, &compileStderr)
	if code != 0 {
		t.Fatalf("Run(compile) exit code = %d, want 0; stderr = %q", code, compileStderr.String())
	}

	var stdout, stderr bytes.Buffer
	code = Run([]string{"read", "subject.md#a"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(read section) exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if got, want := stdout.String(), "## A\n\nA links to [[a-target.md]].\n\n"; got != want {
		t.Fatalf("Run(read section) stdout = %q, want %q", got, want)
	}
	if got, want := stderr.String(), "binding: ratified\nsummary: missing\noutlinks: a-target.md @1\n"; got != want {
		t.Fatalf("Run(read section) stderr = %q, want %q", got, want)
	}
	if strings.Contains(stderr.String(), "b-target.md") {
		t.Fatalf("Run(read section) stderr = %q, want no B section outlink", stderr.String())
	}
}

func TestReadSectionScopesTranscludesToExcerpt(t *testing.T) {
	root := makeCLIVault(t)
	writeCLIFile(t, root, "subject.md", "# Subject\n\n## A\n\nA embeds ![[a-embed.md]].\n\n## B\n\nB embeds ![[b-embed.md]].\n")
	writeCLIFile(t, root, "a-embed.md", "# A Embed\n")
	writeCLIFile(t, root, "b-embed.md", "# B Embed\n")

	var compileStdout, compileStderr bytes.Buffer
	code := Run([]string{"compile"}, &compileStdout, &compileStderr)
	if code != 0 {
		t.Fatalf("Run(compile) exit code = %d, want 0; stderr = %q", code, compileStderr.String())
	}

	var stdout, stderr bytes.Buffer
	code = Run([]string{"read", "subject.md#a"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(read section) exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if got, want := stdout.String(), "## A\n\nA embeds ![[a-embed.md]].\n\n"; got != want {
		t.Fatalf("Run(read section) stdout = %q, want %q", got, want)
	}
	if got, want := stderr.String(), "binding: ratified\nsummary: missing\ntranscludes: a-embed.md @1\n"; got != want {
		t.Fatalf("Run(read section) stderr = %q, want %q", got, want)
	}
	if strings.Contains(stderr.String(), "b-embed.md") {
		t.Fatalf("Run(read section) stderr = %q, want no B section transclude", stderr.String())
	}
}

func TestReadSectionFiltersInlinksByAnchor(t *testing.T) {
	root := makeCLIVault(t)
	writeCLIFile(t, root, "x.md", "# X\n\n## Foo\n\nFoo body.\n\n## Bar\n\nBar body.\n")
	writeCLIFile(t, root, "y.md", "# Y\n\nLinks to [[x.md#foo]].\n")
	writeCLIFile(t, root, "z.md", "# Z\n\nLinks to [[x.md#bar]].\n")

	var compileStdout, compileStderr bytes.Buffer
	code := Run([]string{"compile"}, &compileStdout, &compileStderr)
	if code != 0 {
		t.Fatalf("Run(compile) exit code = %d, want 0; stderr = %q", code, compileStderr.String())
	}

	var stdout, stderr bytes.Buffer
	code = Run([]string{"read", "x.md#foo"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(read section) exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if got, want := stdout.String(), "## Foo\n\nFoo body.\n\n"; got != want {
		t.Fatalf("Run(read section) stdout = %q, want %q", got, want)
	}
	if got, want := stderr.String(), "binding: ratified\nsummary: missing\ninlinks: y.md @2\n"; got != want {
		t.Fatalf("Run(read section) stderr = %q, want %q", got, want)
	}
	if strings.Contains(stderr.String(), "z.md") {
		t.Fatalf("Run(read section) stderr = %q, want no non-matching section inlink", stderr.String())
	}
}

func TestReadSectionInlinksFallbackCanUseFileScope(t *testing.T) {
	m := manifest.Manifest{
		SchemaVersion: manifest.CurrentSchemaVersion,
		Entries: []manifest.Entry{
			{
				Key:      "x.md",
				Headings: []manifest.Heading{{Level: 2, Text: "Foo", Slug: "foo"}},
				Links: manifest.Links{
					In: []manifest.InLink{
						{Source: "y.md", Type: "wiki", Anchor: "foo"},
						{Source: "z.md", Type: "wiki", Anchor: "bar"},
					},
				},
			},
			{Key: "y.md"},
			{Key: "z.md"},
		},
	}

	lines := readSectionLinkSurfaceLines(m, "x.md", "foo", []byte("## Foo\n\nBody.\n"), sectionInlinksFileScoped)
	if got, want := strings.Join(lines, "\n"), "inlinks: y.md @2, z.md @3"; got != want {
		t.Fatalf("readSectionLinkSurfaceLines(file-scoped fallback) = %q, want %q", got, want)
	}
}

func TestReadWholeFileLinkSurfaceUnchangedForAnchoredInlinks(t *testing.T) {
	root := makeCLIVault(t)
	writeCLIFile(t, root, "x.md", "# X\n\n## Foo\n\nFoo body.\n\n## Bar\n\nBar body.\n")
	writeCLIFile(t, root, "y.md", "# Y\n\nLinks to [[x.md#foo]].\n")
	writeCLIFile(t, root, "z.md", "# Z\n\nLinks to [[x.md#bar]].\n")

	var compileStdout, compileStderr bytes.Buffer
	code := Run([]string{"compile"}, &compileStdout, &compileStderr)
	if code != 0 {
		t.Fatalf("Run(compile) exit code = %d, want 0; stderr = %q", code, compileStderr.String())
	}

	var stdout, stderr bytes.Buffer
	code = Run([]string{"read", "x.md"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(read whole file) exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if got, want := stderr.String(), "binding: ratified\nsummary: missing\ninlinks: y.md @2, z.md @3\n"; got != want {
		t.Fatalf("Run(read whole file) stderr = %q, want %q", got, want)
	}
}

func TestReadEmptySectionWithNoLinksEmitsOnlyBinding(t *testing.T) {
	root := makeCLIVault(t)
	writeCLIFile(t, root, "subject.md", "# Subject\n\n## Empty Section\n\n## Next\n\nLinks to [[target.md]].\n")
	writeCLIFile(t, root, "target.md", "# Target\n")

	var compileStdout, compileStderr bytes.Buffer
	code := Run([]string{"compile"}, &compileStdout, &compileStderr)
	if code != 0 {
		t.Fatalf("Run(compile) exit code = %d, want 0; stderr = %q", code, compileStderr.String())
	}

	var stdout, stderr bytes.Buffer
	code = Run([]string{"read", "subject.md#empty-section"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(read empty section) exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if got, want := stdout.String(), "## Empty Section\n\n"; got != want {
		t.Fatalf("Run(read empty section) stdout = %q, want %q", got, want)
	}
	if got, want := stderr.String(), "binding: ratified\nsummary: missing\n"; got != want {
		t.Fatalf("Run(read empty section) stderr = %q, want %q", got, want)
	}
}

func TestReadNumericReferenceFailsWithStaleManifestMessageForMissingFile(t *testing.T) {
	root := makeCLIVault(t)
	writeCLIFile(t, root, "note.md", "# Note\n\nSummary.\n")

	var compileStdout, compileStderr bytes.Buffer
	code := Run([]string{"compile"}, &compileStdout, &compileStderr)
	if code != 0 {
		t.Fatalf("Run(compile) exit code = %d, want 0; stderr = %q", code, compileStderr.String())
	}
	if err := os.Remove(filepath.Join(root, "note.md")); err != nil {
		t.Fatalf("remove note.md: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code = Run([]string{"read", "@1"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("Run(read @1) exit code = %d, want 1", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("Run(read @1) stdout = %q, want empty", stdout.String())
	}
	assertCLIErrorToken(t, stderr.String(), "read", "manifest-stale")
	for _, want := range []string{
		"entry 1's file `note.md` no longer exists",
		"run: memento compile && memento brief",
		"note: entry numbers will likely shift after compile.",
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("Run(read @1) stderr = %q, want %q", stderr.String(), want)
		}
	}
}

func TestReadNumericReferenceWarnsWhenManifestHashDiffersFromBrief(t *testing.T) {
	root := makeCLIVault(t)
	writeCLIFile(t, root, "note.md", "# Note\n\nSummary.\n")

	var compileStdout, compileStderr bytes.Buffer
	code := Run([]string{"compile"}, &compileStdout, &compileStderr)
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
	code = Run([]string{"read", "@1"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(read @1) exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if want := "# Note\n\nSummary.\n"; stdout.String() != want {
		t.Fatalf("Run(read @1) stdout = %q, want %q", stdout.String(), want)
	}
	if !strings.HasPrefix(stderr.String(), "binding: ratified\n") {
		t.Fatalf("Run(read @1) stderr = %q, want binding first", stderr.String())
	}
	if want := "warn: manifest changed since last brief, numbers may not match your view"; !strings.Contains(stderr.String(), want) {
		t.Fatalf("Run(read @1) stderr = %q, want %q", stderr.String(), want)
	}
}

func TestBriefManifestHashReadsFrontmatterField(t *testing.T) {
	tests := []struct {
		name string
		data string
		want string
		ok   bool
	}{
		{
			name: "after mode",
			data: "---\nmode: read-only\nmanifest: sha256:abc123\n---\n# Brief\n",
			want: "sha256:abc123",
			ok:   true,
		},
		{
			name: "before mode",
			data: "---\nmanifest: sha256:def456\nmode: read-only\n---\n# Brief\n",
			want: "sha256:def456",
			ok:   true,
		},
		{
			name: "carriage returns",
			data: "---\r\nmode: read-only\r\nmanifest: sha256:crlf\r\n---\r\n# Brief\r\n",
			want: "sha256:crlf",
			ok:   true,
		},
		{
			name: "missing field",
			data: "---\nmode: read-only\n---\n# Brief\n",
			ok:   false,
		},
		{
			name: "legacy leading comment",
			data: "<!-- manifest: sha256:old -->\n---\nmode: read-only\n---\n# Brief\n",
			ok:   false,
		},
		{
			name: "outside frontmatter",
			data: "---\nmode: read-only\n---\nmanifest: sha256:outside\n",
			ok:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := briefManifestHash([]byte(tt.data))
			if ok != tt.ok || got != tt.want {
				t.Fatalf("briefManifestHash() = %q, %v; want %q, %v", got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestReadNumericReferenceSkipsHashCheckWhenBriefIsMissing(t *testing.T) {
	root := makeCLIVault(t)
	writeCLIFile(t, root, "note.md", "# Note\n\nSummary.\n")

	var compileStdout, compileStderr bytes.Buffer
	code := Run([]string{"compile"}, &compileStdout, &compileStderr)
	if code != 0 {
		t.Fatalf("Run(compile) exit code = %d, want 0; stderr = %q", code, compileStderr.String())
	}
	if err := os.Remove(filepath.Join(root, "_memento", "brief.md")); err != nil {
		t.Fatalf("remove brief: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code = Run([]string{"read", "@1"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(read @1) exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if got, want := stderr.String(), "binding: ratified\nsummary: missing\n"; got != want {
		t.Fatalf("Run(read @1) stderr = %q, want %q", got, want)
	}
	if want := "# Note\n\nSummary.\n"; stdout.String() != want {
		t.Fatalf("Run(read @1) stdout = %q, want %q", stdout.String(), want)
	}
}

func TestReadBareDigitPathReadsVaultFile(t *testing.T) {
	root := makeCLIVault(t)
	writeCLIFile(t, root, "5.md", "# Five\n\nPath note.\n")

	var stdout, stderr bytes.Buffer
	code := Run([]string{"read", "5.md"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(read 5.md) exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	wantStderr := "binding: ratified\nnote: no manifest; link surface unavailable. run: memento compile\n"
	if got := stderr.String(); got != wantStderr {
		t.Fatalf("Run(read 5.md) stderr = %q, want %q", got, wantStderr)
	}
	if want := "# Five\n\nPath note.\n"; stdout.String() != want {
		t.Fatalf("Run(read 5.md) stdout = %q, want %q", stdout.String(), want)
	}
}

func TestReadInvalidNumericReferenceFailsCleanly(t *testing.T) {
	makeCLIVault(t)

	for _, target := range []string{"@", "@abc", "@abc#target-heading"} {
		t.Run(target, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := Run([]string{"read", target}, &stdout, &stderr)
			if code != 1 {
				t.Fatalf("Run(read %s) exit code = %d, want 1", target, code)
			}
			if stdout.Len() != 0 {
				t.Fatalf("Run(read %s) stdout = %q, want empty", target, stdout.String())
			}
			assertCLIErrorToken(t, stderr.String(), "read", "invalid-entry-reference")
			if want := "entry reference must be @ followed by a number: " + target; !strings.Contains(stderr.String(), want) {
				t.Fatalf("Run(read %s) stderr = %q, want %q", target, stderr.String(), want)
			}
		})
	}
}

func TestReadPrintsRequestedSection(t *testing.T) {
	root := makeCLIVault(t)
	writeCLIFile(t, root, "spec.md", "# Spec\n\n## Target Heading\n\nTarget content.\n\n## Next\n\nOther content.\n")

	var stdout, stderr bytes.Buffer
	code := Run([]string{"read", "spec.md#target-heading"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(read section) exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	wantStderr := "binding: ratified\nnote: no manifest; link surface unavailable. run: memento compile\n"
	if got := stderr.String(); got != wantStderr {
		t.Fatalf("Run(read section) stderr = %q, want %q", got, wantStderr)
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
	code := Run([]string{"read", "spec.md#missing"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("Run(read unknown section) exit code = %d, want 1", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("Run(read unknown section) stdout = %q, want empty", stdout.String())
	}
	assertCLIErrorToken(t, stderr.String(), "read", "section-not-found")
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
			code := Run([]string{"read", key}, &stdout, &stderr)
			if code != 1 {
				t.Fatalf("Run(read %s) exit code = %d, want 1", key, code)
			}
			if stdout.Len() != 0 {
				t.Fatalf("Run(read %s) stdout = %q, want empty", key, stdout.String())
			}
			assertCLIErrorToken(t, stderr.String(), "read", "key-not-found")
			if !strings.Contains(stderr.String(), "not found") {
				t.Fatalf("Run(read %s) stderr = %q, want not found message", key, stderr.String())
			}
		})
	}
}

func TestReadSuggestsNearMatchesForMissingKey(t *testing.T) {
	root := makeCLIVault(t)
	writeCLIFile(t, root, "learnings/google-search-grounding.md", "# Google Search Grounding\n\nBody.\n")

	var compileStdout, compileStderr bytes.Buffer
	code := Run([]string{"compile"}, &compileStdout, &compileStderr)
	if code != 0 {
		t.Fatalf("Run(compile) exit code = %d, want 0; stderr = %q", code, compileStderr.String())
	}

	var stdout, stderr bytes.Buffer
	code = Run([]string{"read", "google-search-grounding"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("Run(read missing basename) exit code = %d, want 1", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("Run(read missing basename) stdout = %q, want empty", stdout.String())
	}
	assertCLIErrorToken(t, stderr.String(), "read", "key-not-found")
	want := "did you mean: learnings/google-search-grounding.md (@1)?"
	if !strings.Contains(stderr.String(), want) {
		t.Fatalf("Run(read missing basename) stderr = %q, want %q", stderr.String(), want)
	}
}

func TestReadSuggestsUpToThreeCaseInsensitiveNearMatches(t *testing.T) {
	root := makeCLIVault(t)
	writeCLIFile(t, root, "a/Google-Search-Grounding.md", "# A\n")
	writeCLIFile(t, root, "b/google-search-grounding.md", "# B\n")
	writeCLIFile(t, root, "c/GOOGLE-SEARCH-GROUNDING.md", "# C\n")
	writeCLIFile(t, root, "d/google-search-grounding.md", "# D\n")

	var compileStdout, compileStderr bytes.Buffer
	code := Run([]string{"compile"}, &compileStdout, &compileStderr)
	if code != 0 {
		t.Fatalf("Run(compile) exit code = %d, want 0; stderr = %q", code, compileStderr.String())
	}

	var stdout, stderr bytes.Buffer
	code = Run([]string{"read", "google-search-grounding"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("Run(read missing basename) exit code = %d, want 1", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("Run(read missing basename) stdout = %q, want empty", stdout.String())
	}
	want := "did you mean: a/Google-Search-Grounding.md (@1), b/google-search-grounding.md (@2), c/GOOGLE-SEARCH-GROUNDING.md (@3)?"
	if !strings.Contains(stderr.String(), want) {
		t.Fatalf("Run(read missing basename) stderr = %q, want %q", stderr.String(), want)
	}
	if strings.Contains(stderr.String(), "d/google-search-grounding.md") {
		t.Fatalf("Run(read missing basename) stderr = %q, want at most three suggestions", stderr.String())
	}
}

func TestReadMissingKeyKeepsTerseErrorWithoutSuggestion(t *testing.T) {
	root := makeCLIVault(t)
	writeCLIFile(t, root, "other.md", "# Other\n")

	var compileStdout, compileStderr bytes.Buffer
	code := Run([]string{"compile"}, &compileStdout, &compileStderr)
	if code != 0 {
		t.Fatalf("Run(compile) exit code = %d, want 0; stderr = %q", code, compileStderr.String())
	}

	var stdout, stderr bytes.Buffer
	code = Run([]string{"read", "missing"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("Run(read missing) exit code = %d, want 1", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("Run(read missing) stdout = %q, want empty", stdout.String())
	}
	assertCLIErrorToken(t, stderr.String(), "read", "key-not-found")
	if strings.Contains(stderr.String(), "did you mean") {
		t.Fatalf("Run(read missing) stderr = %q, want no suggestion", stderr.String())
	}
}

func TestReadMissingKeyIgnoresStaleManifestSuggestion(t *testing.T) {
	root := makeCLIVault(t)
	writeCLIFile(t, root, "learnings/google-search-grounding.md", "# Google Search Grounding\n\nBody.\n")

	var compileStdout, compileStderr bytes.Buffer
	code := Run([]string{"compile"}, &compileStdout, &compileStderr)
	if code != 0 {
		t.Fatalf("Run(compile) exit code = %d, want 0; stderr = %q", code, compileStderr.String())
	}
	if err := os.Remove(filepath.Join(root, "learnings", "google-search-grounding.md")); err != nil {
		t.Fatalf("remove stale manifest target: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code = Run([]string{"read", "google-search-grounding"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("Run(read stale basename) exit code = %d, want 1", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("Run(read stale basename) stdout = %q, want empty", stdout.String())
	}
	assertCLIErrorToken(t, stderr.String(), "read", "key-not-found")
	if strings.Contains(stderr.String(), "did you mean") {
		t.Fatalf("Run(read stale basename) stderr = %q, want no suggestion", stderr.String())
	}
}

func TestReadRejectsTraversalKey(t *testing.T) {
	makeCLIVault(t)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"read", "../outside.md"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("Run(read traversal) exit code = %d, want 1", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("Run(read traversal) stdout = %q, want empty", stdout.String())
	}
	assertCLIErrorToken(t, stderr.String(), "read", "invalid-key")
	if !strings.Contains(stderr.String(), "invalid key") {
		t.Fatalf("Run(read traversal) stderr = %q, want invalid key message", stderr.String())
	}
}

func TestWriteCreatesMarkdownFromStdin(t *testing.T) {
	root := makeCLIVault(t)
	body := "# New\n\nDurable note.\n"

	var stdout, stderr bytes.Buffer
	code := RunWithInput(
		[]string{"write", "notes/new.md"},
		strings.NewReader(body),
		&stdout,
		&stderr,
	)
	if code != 0 {
		t.Fatalf("Run(write create) exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("Run(write create) stdout = %q, want empty", stdout.String())
	}
	if got, want := stderr.String(), writeStatusLine(t, root, "notes/new.md", len(body), note.OperationAppend)+writeNewTopLevelDirWarning("notes")+compiledStatusLine(1); got != want {
		t.Fatalf("Run(write create) stderr = %q, want %q", got, want)
	}

	if got := readCLIFile(t, root, "notes/new.md"); got != body {
		t.Fatalf("written note = %q, want %q", got, body)
	}
	manifest := readCLIFile(t, root, ".memento/manifest.json")
	if !strings.Contains(manifest, `"key": "notes/new.md"`) {
		t.Fatalf("manifest after write = %q, want new note entry", manifest)
	}
	brief := readCLIFile(t, root, "_memento/brief.md")
	if !strings.Contains(brief, "key: `notes/new.md` | mode: `append-only`") {
		t.Fatalf("brief after write = %q, want new note entry", brief)
	}
}

func TestWriteDoesNotPrintWritingGuideReminder(t *testing.T) {
	root := makeCLIVault(t)
	writeCLIFile(t, root, "_memento/writing.md", "---\nmode: read-only\n---\n# Writing\n\nUse judgement.\n")
	body := "# New\n\nDurable note.\n"

	var stdout, stderr bytes.Buffer
	code := RunWithInput(
		[]string{"write", "notes/new.md"},
		strings.NewReader(body),
		&stdout,
		&stderr,
	)
	if code != 0 {
		t.Fatalf("Run(write create) exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if strings.Contains(stderr.String(), "writing.md") {
		t.Fatalf("Run(write create) stderr = %q, want no writing guide reminder", stderr.String())
	}
}

func TestWriteWarnsWhenCreatingNewTopLevelVaultDirectory(t *testing.T) {
	root := makeCLIVault(t)
	body := "# New\n\nDurable note.\n"

	var stdout, stderr bytes.Buffer
	code := RunWithInput(
		[]string{"write", "learnings/new.md"},
		strings.NewReader(body),
		&stdout,
		&stderr,
	)
	if code != 0 {
		t.Fatalf("Run(write new top-level) exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("Run(write new top-level) stdout = %q, want empty", stdout.String())
	}
	want := writeStatusLine(t, root, "learnings/new.md", len(body), note.OperationAppend) +
		writeNewTopLevelDirWarning("learnings") +
		compiledStatusLine(1)
	if got := stderr.String(); got != want {
		t.Fatalf("Run(write new top-level) stderr = %q, want %q", got, want)
	}
}

func TestWriteDoesNotWarnWhenWritingIntoExistingTopLevelVaultDirectory(t *testing.T) {
	root := makeCLIVault(t)
	writeCLIFile(t, root, "learnings/existing.md", "# Existing\n\nAlready indexed.\n")
	var compileStdout, compileStderr bytes.Buffer
	code := Run([]string{"compile"}, &compileStdout, &compileStderr)
	if code != 0 {
		t.Fatalf("Run(compile) exit code = %d, want 0; stderr = %q", code, compileStderr.String())
	}

	body := "# New\n\nDurable note.\n"
	var stdout, stderr bytes.Buffer
	code = RunWithInput(
		[]string{"write", "learnings/new.md"},
		strings.NewReader(body),
		&stdout,
		&stderr,
	)
	if code != 0 {
		t.Fatalf("Run(write existing top-level) exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("Run(write existing top-level) stdout = %q, want empty", stdout.String())
	}
	if got, want := stderr.String(), writeSuccessStderr(t, root, "learnings/new.md", len(body), note.OperationAppend, 2); got != want {
		t.Fatalf("Run(write existing top-level) stderr = %q, want %q", got, want)
	}
}

func TestWriteDoesNotWarnForRootLevelFile(t *testing.T) {
	root := makeCLIVault(t)
	body := "# Root\n\nDurable note.\n"

	var stdout, stderr bytes.Buffer
	code := RunWithInput(
		[]string{"write", "root.md"},
		strings.NewReader(body),
		&stdout,
		&stderr,
	)
	if code != 0 {
		t.Fatalf("Run(write root-level) exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("Run(write root-level) stdout = %q, want empty", stdout.String())
	}
	if got, want := stderr.String(), writeSuccessStderr(t, root, "root.md", len(body), note.OperationAppend, 1); got != want {
		t.Fatalf("Run(write root-level) stderr = %q, want %q", got, want)
	}
}

func TestWriteAppendsMarkdownFromStdin(t *testing.T) {
	root := makeCLIVault(t)
	writeCLIFile(t, root, "note.md", "# Note\n\nExisting.\n")
	body := "\nAppended.\n"

	var stdout, stderr bytes.Buffer
	code := RunWithInput(
		[]string{"write", "note.md"},
		strings.NewReader(body),
		&stdout,
		&stderr,
	)
	if code != 0 {
		t.Fatalf("Run(write append) exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("Run(write append) stdout = %q, want empty", stdout.String())
	}
	if got, want := stderr.String(), writeSuccessStderr(t, root, "note.md", len(body), note.OperationAppend, 1); got != want {
		t.Fatalf("Run(write append) stderr = %q, want %q", got, want)
	}

	want := "# Note\n\nExisting.\n\nAppended.\n"
	if got := readCLIFile(t, root, "note.md"); got != want {
		t.Fatalf("appended note = %q, want %q", got, want)
	}
	manifest := readCLIFile(t, root, ".memento/manifest.json")
	if !strings.Contains(manifest, `"summary": "Existing."`) {
		t.Fatalf("manifest after append = %q, want appended note compiled", manifest)
	}
}

func TestWriteOverwritesMarkdownFromStdinWithExplicitFlag(t *testing.T) {
	root := makeCLIVault(t)
	initCLIGit(t, root)
	writeCLIFile(t, root, "note.md", "---\nmode: living\n---\n# Note\n\nExisting.\n")
	commitCLIGit(t, root)
	body := "---\nmode: living\n---\n# Note\n\nReplacement.\n"

	var stdout, stderr bytes.Buffer
	code := RunWithInput(
		[]string{"write", "--overwrite", "note.md"},
		strings.NewReader(body),
		&stdout,
		&stderr,
	)
	if code != 0 {
		t.Fatalf("Run(write --overwrite) exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("Run(write --overwrite) stdout = %q, want empty", stdout.String())
	}
	if got, want := stderr.String(), writeSuccessStderr(t, root, "note.md", len(body), note.OperationOverwrite, 1); got != want {
		t.Fatalf("Run(write --overwrite) stderr = %q, want %q", got, want)
	}

	if got := readCLIFile(t, root, "note.md"); got != body {
		t.Fatalf("overwritten note = %q, want %q", got, body)
	}
}

func TestCompileAndWriteReportSameCompiledLineFormat(t *testing.T) {
	compileRoot := makeCLIVault(t)
	writeCLIFile(t, compileRoot, "note.md", "# Note\n\nSummary.\n")

	var compileStdout, compileStderr bytes.Buffer
	code := Run([]string{"compile"}, &compileStdout, &compileStderr)
	if code != 0 {
		t.Fatalf("Run(compile) exit code = %d, want 0; stderr = %q", code, compileStderr.String())
	}
	if compileStdout.Len() != 0 {
		t.Fatalf("Run(compile) stdout = %q, want empty", compileStdout.String())
	}

	makeCLIVault(t)
	var writeStdout, writeStderr bytes.Buffer
	code = RunWithInput(
		[]string{"write", "note.md"},
		strings.NewReader("# Note\n\nSummary.\n"),
		&writeStdout,
		&writeStderr,
	)
	if code != 0 {
		t.Fatalf("Run(write) exit code = %d, want 0; stderr = %q", code, writeStderr.String())
	}
	if writeStdout.Len() != 0 {
		t.Fatalf("Run(write) stdout = %q, want empty", writeStdout.String())
	}

	writeLines := strings.SplitAfter(writeStderr.String(), "\n")
	if len(writeLines) != 3 || writeLines[2] != "" {
		t.Fatalf("write stderr = %q, want wrote line plus compiled line", writeStderr.String())
	}
	if got, want := writeLines[1], compileStderr.String(); got != want {
		t.Fatalf("write compiled line = %q, want compile stderr %q", got, want)
	}
	if got, want := compileStderr.String(), compiledStatusLine(1); got != want {
		t.Fatalf("compiled line = %q, want %q", got, want)
	}
}

func TestWriteFailureDoesNotRecompile(t *testing.T) {
	root := makeCLIVault(t)
	initCLIGit(t, root)
	original := "---\nmode: append-only\n---\n# Note\n\nExisting.\n"
	writeCLIFile(t, root, "note.md", original)
	commitCLIGit(t, root)
	writeCLIFile(t, root, ".memento/manifest.json", "sentinel manifest\n")

	var stdout, stderr bytes.Buffer
	code := RunWithInput(
		[]string{"write", "--overwrite", "note.md"},
		strings.NewReader("# Replacement\n"),
		&stdout,
		&stderr,
	)
	if code != 1 {
		t.Fatalf("Run(write --overwrite append-only) exit code = %d, want 1", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("Run(write --overwrite append-only) stdout = %q, want empty", stdout.String())
	}
	assertCLIErrorToken(t, stderr.String(), "write", "mode-rejects-write")
	if got := readCLIFile(t, root, ".memento/manifest.json"); got != "sentinel manifest\n" {
		t.Fatalf("manifest changed after rejected write: %q", got)
	}
	if got := readCLIFile(t, root, "note.md"); got != original {
		t.Fatalf("append-only note changed after rejected overwrite: %q", got)
	}
}

func TestWritePersistsBodyButReturnsPartialSuccessWhenRecompileFails(t *testing.T) {
	root := makeCLIVault(t)
	previous := writeCompileArtifactsAfterWrite
	writeCompileArtifactsAfterWrite = func(vault.Vault) ([]manifest.Warning, int, error) {
		return nil, 0, errors.New("injected compile failure")
	}
	t.Cleanup(func() {
		writeCompileArtifactsAfterWrite = previous
	})

	body := "# Persisted\n\nBody.\n"
	var stdout, stderr bytes.Buffer
	code := RunWithInput(
		[]string{"write", "persisted.md"},
		strings.NewReader(body),
		&stdout,
		&stderr,
	)
	if code != 3 {
		t.Fatalf("Run(write compile failure) exit code = %d, want 3", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("Run(write compile failure) stdout = %q, want empty", stdout.String())
	}
	for _, want := range []string{
		writeStatusLine(t, root, "persisted.md", len(body), note.OperationAppend),
		"memento write: warning: write succeeded but recompile failed; run 'memento compile' to refresh the manifest:",
		"injected compile failure",
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("Run(write compile failure) stderr = %q, want %q", stderr.String(), want)
		}
	}
	if got := readCLIFile(t, root, "persisted.md"); got != "# Persisted\n\nBody.\n" {
		t.Fatalf("written body after recompile failure = %q", got)
	}
}

func TestSequentialWritesLeaveManifestAtFinalState(t *testing.T) {
	root := makeCLIVault(t)
	initCLIGit(t, root)

	for _, body := range []string{"# Note\n\nFirst.\n", "# Note\n\nSecond.\n"} {
		var stdout, stderr bytes.Buffer
		code := RunWithInput(
			[]string{"write", "--overwrite", "note.md"},
			strings.NewReader(body),
			&stdout,
			&stderr,
		)
		if code != 0 {
			t.Fatalf("Run(write --overwrite note.md) exit code = %d, want 0; stderr = %q", code, stderr.String())
		}
	}

	manifest := readCLIFile(t, root, ".memento/manifest.json")
	if strings.Contains(manifest, `"summary": "First."`) || !strings.Contains(manifest, `"summary": "Second."`) {
		t.Fatalf("manifest after sequential writes = %q, want final summary only", manifest)
	}
}

func TestWriteOverwriteRejectsRatifiedAppendOnlyMode(t *testing.T) {
	root := makeCLIVault(t)
	initCLIGit(t, root)
	original := "---\nmode: append-only\n---\n# Note\n\nExisting.\n"
	writeCLIFile(t, root, "note.md", original)
	commitCLIGit(t, root)

	var stdout, stderr bytes.Buffer
	code := RunWithInput(
		[]string{"write", "--overwrite", "note.md"},
		strings.NewReader("# Replacement\n"),
		&stdout,
		&stderr,
	)
	if code != 1 {
		t.Fatalf("Run(write --overwrite append-only) exit code = %d, want 1", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("Run(write --overwrite append-only) stdout = %q, want empty", stdout.String())
	}
	assertCLIErrorToken(t, stderr.String(), "write", "mode-rejects-write")
	if got := readCLIFile(t, root, "note.md"); got != original {
		t.Fatalf("append-only note changed after rejected overwrite: %q", got)
	}
}

func TestWriteRejectsTraversalKey(t *testing.T) {
	makeCLIVault(t)

	var stdout, stderr bytes.Buffer
	code := RunWithInput(
		[]string{"write", "../outside.md"},
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
	assertCLIErrorToken(t, stderr.String(), "write", "invalid-key")
	if !strings.Contains(stderr.String(), "invalid key") {
		t.Fatalf("Run(write traversal) stderr = %q, want invalid key message", stderr.String())
	}
}

func TestWriteRejectsVaultPrefixedKeyWithInvalidKeyToken(t *testing.T) {
	repo := t.TempDir()
	root := filepath.Join(repo, "agents-memory")
	if err := os.MkdirAll(filepath.Join(root, vault.MarkerDirName), 0o755); err != nil {
		t.Fatalf("mkdir vault marker: %v", err)
	}
	chdirCLI(t, repo)

	var stdout, stderr bytes.Buffer
	code := RunWithInput(
		[]string{"write", "agents-memory/learnings/x.md"},
		strings.NewReader("# Learning\n"),
		&stdout,
		&stderr,
	)
	if code != 1 {
		t.Fatalf("Run(write vault-prefixed) exit code = %d, want 1", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("Run(write vault-prefixed) stdout = %q, want empty", stdout.String())
	}
	assertCLIErrorToken(t, stderr.String(), "write", "invalid-key")
	for _, want := range []string{"key is vault-relative, not repo-relative", `did you mean "learnings/x.md"?`} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("Run(write vault-prefixed) stderr = %q, want %q", stderr.String(), want)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "agents-memory", "learnings", "x.md")); !os.IsNotExist(err) {
		t.Fatalf("vault-prefixed nested file was created; stat err = %v", err)
	}
}

func TestWriteRejectsDifferentlyCasedVaultPrefixedKeyWithInvalidKeyToken(t *testing.T) {
	repo := t.TempDir()
	root := filepath.Join(repo, "agents-memory")
	if err := os.MkdirAll(filepath.Join(root, vault.MarkerDirName), 0o755); err != nil {
		t.Fatalf("mkdir vault marker: %v", err)
	}
	chdirCLI(t, repo)

	var stdout, stderr bytes.Buffer
	code := RunWithInput(
		[]string{"write", "AGENTS-MEMORY/learnings/x.md"},
		strings.NewReader("# Learning\n"),
		&stdout,
		&stderr,
	)
	if code != 1 {
		t.Fatalf("Run(write vault-prefixed) exit code = %d, want 1", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("Run(write vault-prefixed) stdout = %q, want empty", stdout.String())
	}
	assertCLIErrorToken(t, stderr.String(), "write", "invalid-key")
	for _, want := range []string{"key is vault-relative, not repo-relative", `did you mean "learnings/x.md"?`} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("Run(write vault-prefixed) stderr = %q, want %q", stderr.String(), want)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "AGENTS-MEMORY", "learnings", "x.md")); !os.IsNotExist(err) {
		t.Fatalf("differently-cased vault-prefixed nested file was created; stat err = %v", err)
	}
}

func TestCLIErrorTokensForAdditionalDeterministicPaths(t *testing.T) {
	t.Run("invalid arguments", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := Run([]string{"read"}, &stdout, &stderr)
		if code != 2 {
			t.Fatalf("Run(read) exit code = %d, want 2", code)
		}
		assertCLIErrorToken(t, stderr.String(), "read", "invalid-arguments")
	})

	t.Run("vault not found", func(t *testing.T) {
		root := t.TempDir()
		chdirCLI(t, root)
		var stdout, stderr bytes.Buffer
		code := Run([]string{"read", "note.md"}, &stdout, &stderr)
		if code != 1 {
			t.Fatalf("Run(read missing-vault) exit code = %d, want 1", code)
		}
		assertCLIErrorToken(t, stderr.String(), "read", "vault-not-found")
	})

	t.Run("numeric out of range", func(t *testing.T) {
		root := makeCLIVault(t)
		writeCLIFile(t, root, "note.md", "# Note\n")
		var compileStdout, compileStderr bytes.Buffer
		code := Run([]string{"compile"}, &compileStdout, &compileStderr)
		if code != 0 {
			t.Fatalf("Run(compile) exit code = %d, want 0; stderr = %q", code, compileStderr.String())
		}

		var stdout, stderr bytes.Buffer
		code = Run([]string{"read", "@2"}, &stdout, &stderr)
		if code != 1 {
			t.Fatalf("Run(read @2) exit code = %d, want 1", code)
		}
		assertCLIErrorToken(t, stderr.String(), "read", "numeric-out-of-range")
	})

	t.Run("ignore file invalid", func(t *testing.T) {
		root := makeCLIVault(t)
		writeCLIFile(t, root, ".mementoignore", "!unsupported\n")
		var stdout, stderr bytes.Buffer
		code := Run([]string{"compile"}, &stdout, &stderr)
		if code != 1 {
			t.Fatalf("Run(compile invalid ignore) exit code = %d, want 1", code)
		}
		assertCLIErrorToken(t, stderr.String(), "compile", "ignore-file-invalid")
	})

	t.Run("manifest invalid", func(t *testing.T) {
		root := makeCLIVault(t)
		writeCLIFile(t, root, ".memento/manifest.json", "{not json")
		var stdout, stderr bytes.Buffer
		code := Run([]string{"orient"}, &stdout, &stderr)
		if code != 1 {
			t.Fatalf("Run(orient invalid manifest) exit code = %d, want 1", code)
		}
		assertCLIErrorToken(t, stderr.String(), "orient", "manifest-invalid")
	})

	t.Run("manifest schema unsupported", func(t *testing.T) {
		root := makeCLIVault(t)
		writeCLIFile(t, root, ".memento/manifest.json", `{"entries":[]}`)
		var stdout, stderr bytes.Buffer
		code := Run([]string{"orient"}, &stdout, &stderr)
		if code != 1 {
			t.Fatalf("Run(orient unsupported schema) exit code = %d, want 1", code)
		}
		assertCLIErrorToken(t, stderr.String(), "orient", "manifest-schema-unsupported")
	})

	t.Run("frontmatter invalid", func(t *testing.T) {
		root := makeCLIVault(t)
		writeCLIFile(t, root, "note.md", "---\ntitle\n---\n# Note\n")
		var stdout, stderr bytes.Buffer
		code := RunWithInput([]string{"write", "note.md"}, strings.NewReader("append\n"), &stdout, &stderr)
		if code != 1 {
			t.Fatalf("Run(write invalid frontmatter) exit code = %d, want 1", code)
		}
		assertCLIErrorToken(t, stderr.String(), "write", "frontmatter-invalid")
	})

	t.Run("mode rejects write", func(t *testing.T) {
		root := makeCLIVault(t)
		writeCLIFile(t, root, "frozen.md", "---\nmode: read-only\n---\n# Frozen\n")
		var stdout, stderr bytes.Buffer
		code := RunWithInput([]string{"write", "frozen.md"}, strings.NewReader("append\n"), &stdout, &stderr)
		if code != 1 {
			t.Fatalf("Run(write read-only) exit code = %d, want 1", code)
		}
		assertCLIErrorToken(t, stderr.String(), "write", "mode-rejects-write")
	})
}

func TestCLIHelperErrorsWrapStableSentinels(t *testing.T) {
	t.Run("manifest not found", func(t *testing.T) {
		root := makeCLIVault(t)
		v, err := vault.Open(root)
		if err != nil {
			t.Fatalf("vault.Open() error = %v, want nil", err)
		}
		_, err = readManifest(v)
		if !errors.Is(err, manifest.ErrNotFound) {
			t.Fatalf("readManifest() error = %v, want manifest.ErrNotFound", err)
		}
	})

	t.Run("invalid entry reference", func(t *testing.T) {
		root := makeCLIVault(t)
		v, err := vault.Open(root)
		if err != nil {
			t.Fatalf("vault.Open() error = %v, want nil", err)
		}
		_, _, _, err = readNumberedEntry(v, "abc")
		if !errors.Is(err, ErrInvalidEntryReference) {
			t.Fatalf("readNumberedEntry(abc) error = %v, want ErrInvalidEntryReference", err)
		}
	})

	t.Run("numeric out of range", func(t *testing.T) {
		root := makeCLIVault(t)
		writeCLIFile(t, root, "note.md", "# Note\n")
		v, err := vault.Open(root)
		if err != nil {
			t.Fatalf("vault.Open() error = %v, want nil", err)
		}
		if _, _, err := writeCompileArtifacts(v); err != nil {
			t.Fatalf("writeCompileArtifacts() error = %v, want nil", err)
		}
		_, _, _, err = readNumberedEntry(v, "0")
		if !errors.Is(err, ErrNumericOutOfRange) {
			t.Fatalf("readNumberedEntry(0) error = %v, want ErrNumericOutOfRange", err)
		}
	})

	t.Run("manifest stale", func(t *testing.T) {
		root := makeCLIVault(t)
		writeCLIFile(t, root, "note.md", "# Note\n")
		v, err := vault.Open(root)
		if err != nil {
			t.Fatalf("vault.Open() error = %v, want nil", err)
		}
		if _, _, err := writeCompileArtifacts(v); err != nil {
			t.Fatalf("writeCompileArtifacts() error = %v, want nil", err)
		}
		if err := os.Remove(filepath.Join(root, "note.md")); err != nil {
			t.Fatalf("remove note.md: %v", err)
		}
		_, _, _, err = readNumberedEntry(v, "1")
		if !errors.Is(err, manifest.ErrStale) {
			t.Fatalf("readNumberedEntry(1) error = %v, want manifest.ErrStale", err)
		}
		if errors.Is(err, note.ErrNotFound) {
			t.Fatalf("readNumberedEntry(1) error = %v, should expose manifest staleness instead of note not found", err)
		}
	})
}

func TestWriteDoesNotOfferDeferredMutationFlags(t *testing.T) {
	root := makeCLIVault(t)
	writeCLIFile(t, root, "note.md", "# Note\n\nOriginal.\n")

	for _, args := range [][]string{
		{"write", "--section", "context", "note.md"},
		{"write", "--upsert", "key", "note.md"},
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

func assertCLIErrorToken(t *testing.T, stderr, verb, token string) {
	t.Helper()

	want := "memento " + verb + ": " + token + ":"
	if !strings.Contains(stderr, want) {
		t.Fatalf("stderr = %q, want token prefix %q", stderr, want)
	}
}

func assertRootErrorToken(t *testing.T, stderr, token string) {
	t.Helper()

	want := "memento: " + token + ":"
	if !strings.Contains(stderr, want) {
		t.Fatalf("stderr = %q, want token prefix %q", stderr, want)
	}
}

func compiledStatusLine(count int) string {
	return fmt.Sprintf("compiled: %d entries\n", count)
}

func writeStatusLine(t *testing.T, root, key string, byteCount int, operation note.WriteOperation) string {
	t.Helper()

	return fmt.Sprintf("wrote: %s (%d, %s)\n", resolvedCLIPath(t, root, key), byteCount, operation)
}

func writeSuccessStderr(t *testing.T, root, key string, byteCount int, operation note.WriteOperation, compiledCount int) string {
	t.Helper()

	return writeStatusLine(t, root, key, byteCount, operation) + compiledStatusLine(compiledCount)
}

func writeNewTopLevelDirWarning(segment string) string {
	return fmt.Sprintf("warn: created new top-level vault directory '%s' — confirm this is intentional\n", segment)
}

func resolvedCLIPath(t *testing.T, root, key string) string {
	t.Helper()

	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("resolve root %q: %v", root, err)
	}
	realRoot, err = filepath.Abs(realRoot)
	if err != nil {
		t.Fatalf("abs root %q: %v", realRoot, err)
	}
	return filepath.Join(realRoot, filepath.FromSlash(key))
}

func isCompiledStatusLine(line string) bool {
	var count int
	if _, err := fmt.Sscanf(line, "compiled: %d entries\n", &count); err != nil {
		return false
	}
	return line == compiledStatusLine(count)
}

func makeCLIVault(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".memento"), 0o755); err != nil {
		t.Fatalf("mkdir marker: %v", err)
	}
	chdirCLI(t, root)
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

func initCLIGit(t *testing.T, root string) {
	t.Helper()

	runCLIGit(t, root, "init")
}

func commitCLIGit(t *testing.T, root string) {
	t.Helper()

	runCLIGit(t, root, "add", ".")
	runCLIGit(t, root,
		"-c", "user.name=Memento Test",
		"-c", "user.email=memento-test@example.invalid",
		"commit", "--no-gpg-sign", "-m", "initial",
	)
}

func runCLIGit(t *testing.T, root string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, string(output))
	}
}

func readRepoFile(t *testing.T, relPath string) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		path := filepath.Join(dir, filepath.FromSlash(relPath))
		data, err := os.ReadFile(path)
		if err == nil {
			return string(data)
		}
		if !os.IsNotExist(err) {
			t.Fatalf("read %q: %v", path, err)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("find repo file %q from %q: not found", relPath, dir)
		}
		dir = parent
	}
}

func readmeCurrentVerbUsages(readme string) []string {
	var usages []string
	inList := false
	for _, line := range strings.Split(readme, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "## CLI reference" {
			inList = true
			continue
		}
		if !inList {
			continue
		}
		if strings.TrimSpace(line) == "" {
			if len(usages) > 0 {
				break
			}
			continue
		}
		const prefix = "- `memento "
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		rest := strings.TrimPrefix(line, prefix)
		usage, _, ok := strings.Cut(rest, "`")
		if ok {
			usages = append(usages, "memento "+usage)
		}
	}
	return usages
}

func helpUsageLines(help string) []string {
	var usages []string
	inUsage := false
	for _, line := range strings.Split(help, "\n") {
		if line == "Usage:" {
			inUsage = true
			continue
		}
		if !inUsage {
			continue
		}
		if strings.TrimSpace(line) == "" {
			if len(usages) > 0 {
				break
			}
			continue
		}
		usages = append(usages, strings.TrimSpace(line))
	}
	return usages
}

func commandNamesFromUsages(usages []string) []string {
	var names []string
	for _, usage := range usages {
		fields := strings.Fields(usage)
		if len(fields) >= 2 {
			names = append(names, fields[1])
		}
	}
	return names
}

func helpCommandNames(help string) []string {
	var names []string
	inCommands := false
	for _, line := range strings.Split(help, "\n") {
		if line == "Commands:" {
			inCommands = true
			continue
		}
		if !inCommands {
			continue
		}
		if strings.TrimSpace(line) == "" {
			if len(names) > 0 {
				break
			}
			continue
		}
		fields := strings.Fields(line)
		if len(fields) > 0 {
			names = append(names, fields[0])
		}
	}
	return names
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
