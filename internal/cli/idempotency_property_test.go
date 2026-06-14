package cli

import (
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

const idempotencyPropertyRuns = 40

type idempotencyCase struct {
	Notes          []propertyNote
	HasAgents      bool
	Agents         string
	HasClaude      bool
	Claude         string
	HasGitignore   bool
	Gitignore      string
	HasHook        bool
	Hook           string
	HookExecutable bool
	HasIgnore      bool
	Ignore         string
	HasGuide       bool
	Guide          string
}

type propertyNote struct {
	Path        string
	Title       string
	Summary     string
	Tags        []string
	Mode        string
	Orient      bool
	Updated     string
	Frontmatter bool
	Headings    []string
	LinkTarget  string
}

func TestCompileIdempotencyProperty(t *testing.T) {
	checkIdempotencyProperty(t, "compile", func(seed int64) idempotencyCase {
		return generateIdempotencyCase(seed)
	}, func(base string, c idempotencyCase) string {
		return checkCompileIdempotent(base, c)
	})
}

func TestBriefIdempotencyProperty(t *testing.T) {
	checkIdempotencyProperty(t, "brief", func(seed int64) idempotencyCase {
		return generateIdempotencyCase(seed)
	}, func(base string, c idempotencyCase) string {
		return checkBriefIdempotent(base, c)
	})
}

func TestInitIdempotencyProperty(t *testing.T) {
	checkIdempotencyProperty(t, "init", func(seed int64) idempotencyCase {
		return generateIdempotencyCase(seed)
	}, func(base string, c idempotencyCase) string {
		return checkInitIdempotent(base, c)
	})
}

func checkIdempotencyProperty(t *testing.T, name string, generate func(int64) idempotencyCase, check func(string, idempotencyCase) string) {
	t.Helper()

	base := t.TempDir()
	for seed := int64(0); seed < idempotencyPropertyRuns; seed++ {
		c := generate(seed)
		caseRoot := filepath.Join(base, fmt.Sprintf("%s-seed-%02d", name, seed))
		if failure := check(caseRoot, c); failure != "" {
			shrunk := shrinkIdempotencyCase(c, func(candidate idempotencyCase) string {
				return check(filepath.Join(base, fmt.Sprintf("%s-seed-%02d-shrink", name, seed)), candidate)
			})
			t.Fatalf("%s idempotency property failed for seed %d\nfailure: %s\nminimal counter-example:\n%s", name, seed, failure, formatIdempotencyCase(shrunk))
		}
	}
}

func checkCompileIdempotent(root string, c idempotencyCase) string {
	if err := resetDir(root); err != nil {
		return err.Error()
	}
	if err := writeVaultCase(root, c); err != nil {
		return err.Error()
	}

	if failure := runCLIExpectSuccess([]string{"compile", "--dir", root}, true); failure != "" {
		return "first compile: " + failure
	}
	firstManifest, err := os.ReadFile(filepath.Join(root, ".memento", "manifest.json"))
	if err != nil {
		return fmt.Sprintf("read first manifest: %v", err)
	}
	firstBrief, err := os.ReadFile(filepath.Join(root, "_memento", "brief.md"))
	if err != nil {
		return fmt.Sprintf("read first brief: %v", err)
	}

	if failure := runCLIExpectSuccess([]string{"compile", "--dir", root}, true); failure != "" {
		return "second compile: " + failure
	}
	secondManifest, err := os.ReadFile(filepath.Join(root, ".memento", "manifest.json"))
	if err != nil {
		return fmt.Sprintf("read second manifest: %v", err)
	}
	secondBrief, err := os.ReadFile(filepath.Join(root, "_memento", "brief.md"))
	if err != nil {
		return fmt.Sprintf("read second brief: %v", err)
	}

	if !bytes.Equal(secondManifest, firstManifest) {
		return fmt.Sprintf("manifest changed between compile runs\nfirst:\n%s\nsecond:\n%s", firstManifest, secondManifest)
	}
	if !bytes.Equal(secondBrief, firstBrief) {
		return fmt.Sprintf("brief changed between compile runs\nfirst:\n%s\nsecond:\n%s", firstBrief, secondBrief)
	}
	return ""
}

func checkBriefIdempotent(root string, c idempotencyCase) string {
	if err := resetDir(root); err != nil {
		return err.Error()
	}
	if err := writeVaultCase(root, c); err != nil {
		return err.Error()
	}
	if failure := runCLIExpectSuccess([]string{"compile", "--dir", root}, true); failure != "" {
		return "compile setup: " + failure
	}
	if err := os.Remove(filepath.Join(root, "_memento", "brief.md")); err != nil {
		return fmt.Sprintf("remove generated brief: %v", err)
	}

	var firstOut bytes.Buffer
	if failure := runCLIExpectSuccessWithStdout([]string{"brief", "--dir", root}, &firstOut, false); failure != "" {
		return "first brief: " + failure
	}
	if err := setAllFileTimes(root, fixedSnapshotTime); err != nil {
		return fmt.Sprintf("set first brief mtimes: %v", err)
	}
	firstSnapshot, err := snapshotFiles(root)
	if err != nil {
		return fmt.Sprintf("snapshot after first brief: %v", err)
	}

	var secondOut bytes.Buffer
	if failure := runCLIExpectSuccessWithStdout([]string{"brief", "--dir", root}, &secondOut, false); failure != "" {
		return "second brief: " + failure
	}
	secondSnapshot, err := snapshotFiles(root)
	if err != nil {
		return fmt.Sprintf("snapshot after second brief: %v", err)
	}

	if secondOut.String() != firstOut.String() {
		return fmt.Sprintf("brief stdout changed between runs\nfirst:\n%s\nsecond:\n%s", firstOut.String(), secondOut.String())
	}
	if !reflect.DeepEqual(secondSnapshot, firstSnapshot) {
		return diffSnapshots(firstSnapshot, secondSnapshot)
	}
	return ""
}

func checkInitIdempotent(root string, c idempotencyCase) string {
	if err := resetDir(root); err != nil {
		return err.Error()
	}
	repo := filepath.Join(root, "repo")
	if err := writeInitCase(repo, c); err != nil {
		return err.Error()
	}

	if failure := runCLIInDirExpectSuccess(repo, []string{"init", "--dir", "memory"}); failure != "" {
		return "first init: " + failure
	}
	if err := setAllFileTimes(repo, fixedSnapshotTime); err != nil {
		return fmt.Sprintf("set first init mtimes: %v", err)
	}
	firstSnapshot, err := snapshotFiles(repo)
	if err != nil {
		return fmt.Sprintf("snapshot after first init: %v", err)
	}

	if failure := runCLIInDirExpectSuccess(repo, []string{"init", "--dir", "memory"}); failure != "" {
		return "second init: " + failure
	}
	secondSnapshot, err := snapshotFiles(repo)
	if err != nil {
		return fmt.Sprintf("snapshot after second init: %v", err)
	}

	if !reflect.DeepEqual(secondSnapshot, firstSnapshot) {
		return diffSnapshots(firstSnapshot, secondSnapshot)
	}
	return ""
}

func generateIdempotencyCase(seed int64) idempotencyCase {
	r := rand.New(rand.NewSource(seed))
	count := r.Intn(8)
	notes := make([]propertyNote, 0, count)
	stems := make([]string, 0, count)
	for i := 0; i < count; i++ {
		folder := ""
		switch r.Intn(4) {
		case 1:
			folder = "notes/"
		case 2:
			folder = "Architecture decision record/"
		case 3:
			folder = "deep/topic/"
		}
		stem := fmt.Sprintf("%s-%02d", propertyWords[(int(seed)+i)%len(propertyWords)], i)
		path := folder + stem + ".md"
		var linkTarget string
		if len(stems) > 0 && r.Intn(2) == 0 {
			linkTarget = stems[r.Intn(len(stems))]
		}
		stems = append(stems, stem)

		tags := []string{}
		for _, tag := range []string{"memento", "brief", "init", "v1"} {
			if r.Intn(4) == 0 {
				tags = append(tags, tag)
			}
		}

		notes = append(notes, propertyNote{
			Path:        path,
			Title:       titleWords(strings.ReplaceAll(stem, "-", " ")),
			Summary:     fmt.Sprintf("Summary for %s.", stem),
			Tags:        tags,
			Mode:        []string{"append-only", "read-only", "living"}[r.Intn(3)],
			Orient:      r.Intn(5) == 0,
			Updated:     fmt.Sprintf("2026-06-%02d", 1+r.Intn(14)),
			Frontmatter: r.Intn(2) == 0,
			Headings:    randomHeadings(r),
			LinkTarget:  linkTarget,
		})
	}

	return idempotencyCase{
		Notes:          notes,
		HasAgents:      r.Intn(2) == 0,
		Agents:         "# Agent Instructions\n\nKeep existing agent rules.\n",
		HasClaude:      r.Intn(4) == 0,
		Claude:         "# Claude Instructions\n\nKeep existing Claude rules.\n",
		HasGitignore:   r.Intn(2) == 0,
		Gitignore:      "build/\n",
		HasHook:        r.Intn(3) == 0,
		Hook:           "#!/bin/sh\nset -eu\n\necho existing\n",
		HookExecutable: r.Intn(2) == 0,
		HasIgnore:      r.Intn(2) == 0,
		Ignore:         "drafts/\n",
		HasGuide:       r.Intn(4) == 0,
		Guide:          "# Local Memento Guide\n\nKeep this exactly.\n",
	}
}

func randomHeadings(r *rand.Rand) []string {
	count := r.Intn(4)
	headings := make([]string, 0, count)
	for i := 0; i < count; i++ {
		headings = append(headings, titleWords(propertyWords[r.Intn(len(propertyWords))]))
	}
	return headings
}

var propertyWords = []string{"alpha", "beta", "gamma", "delta", "epsilon", "zeta", "eta", "theta"}

func writeVaultCase(root string, c idempotencyCase) error {
	if err := os.MkdirAll(filepath.Join(root, ".memento"), 0o755); err != nil {
		return fmt.Errorf("mkdir marker: %w", err)
	}
	if err := writeFile(root, ".mementoignore", "# generated artifacts\n_memento/brief.md\n"); err != nil {
		return err
	}
	for _, note := range c.Notes {
		if err := writeFile(root, note.Path, note.markdown()); err != nil {
			return err
		}
	}
	return nil
}

func writeInitCase(repo string, c idempotencyCase) error {
	if err := os.MkdirAll(filepath.Join(repo, ".git", "hooks"), 0o755); err != nil {
		return fmt.Errorf("mkdir .git/hooks: %w", err)
	}
	if c.HasAgents {
		if err := writeFile(repo, "AGENTS.md", c.Agents); err != nil {
			return err
		}
	}
	if c.HasClaude {
		if err := writeFile(repo, "CLAUDE.md", c.Claude); err != nil {
			return err
		}
	}
	if c.HasGitignore {
		if err := writeFile(repo, ".gitignore", c.Gitignore); err != nil {
			return err
		}
	}
	if c.HasHook {
		if err := writeFile(repo, ".git/hooks/pre-commit", c.Hook); err != nil {
			return err
		}
		if c.HookExecutable {
			if err := os.Chmod(filepath.Join(repo, ".git", "hooks", "pre-commit"), 0o755); err != nil {
				return fmt.Errorf("chmod hook: %w", err)
			}
		}
	}
	if c.HasIgnore {
		if err := writeFile(filepath.Join(repo, "memory"), ".mementoignore", c.Ignore); err != nil {
			return err
		}
	}
	if c.HasGuide {
		if err := writeFile(filepath.Join(repo, "memory"), "_memento/Using Memento.md", c.Guide); err != nil {
			return err
		}
	}
	for _, note := range c.Notes {
		if err := writeFile(filepath.Join(repo, "memory"), note.Path, note.markdown()); err != nil {
			return err
		}
	}
	return nil
}

func (n propertyNote) markdown() string {
	var b strings.Builder
	if n.Frontmatter {
		b.WriteString("---\n")
		fmt.Fprintf(&b, "title: %s\n", n.Title)
		fmt.Fprintf(&b, "summary: %s\n", n.Summary)
		if len(n.Tags) > 0 {
			fmt.Fprintf(&b, "tags: [%s]\n", strings.Join(n.Tags, ", "))
		}
		fmt.Fprintf(&b, "mode: %s\n", n.Mode)
		if n.Orient {
			b.WriteString("orient: true\n")
		}
		fmt.Fprintf(&b, "updated: %s\n", n.Updated)
		b.WriteString("summary_hash: mismatch\n")
		b.WriteString("---\n\n")
	}
	fmt.Fprintf(&b, "# %s\n\n", n.Title)
	fmt.Fprintf(&b, "%s", n.Summary)
	if n.LinkTarget != "" {
		fmt.Fprintf(&b, " See [[%s]].", n.LinkTarget)
	}
	b.WriteString("\n")
	for _, heading := range n.Headings {
		fmt.Fprintf(&b, "\n## %s\n\nBody for %s.\n", heading, heading)
	}
	return b.String()
}

func runCLIExpectSuccess(args []string, wantEmptyStdout bool) string {
	return runCLIExpectSuccessWithStdout(args, nil, wantEmptyStdout)
}

func runCLIExpectSuccessWithStdout(args []string, stdoutSink *bytes.Buffer, wantEmptyStdout bool) string {
	var stdout, stderr bytes.Buffer
	code := Run(args, &stdout, &stderr)
	if stdoutSink != nil {
		*stdoutSink = stdout
	}
	if code != 0 {
		return fmt.Sprintf("exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if stderr.Len() != 0 {
		return fmt.Sprintf("stderr = %q, want empty", stderr.String())
	}
	if wantEmptyStdout && stdout.Len() != 0 {
		return fmt.Sprintf("stdout = %q, want empty", stdout.String())
	}
	return ""
}

func titleWords(text string) string {
	parts := strings.Fields(text)
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
}

func runCLIInDirExpectSuccess(dir string, args []string) string {
	previous, err := os.Getwd()
	if err != nil {
		return fmt.Sprintf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		return fmt.Sprintf("chdir %q: %v", dir, err)
	}
	defer func() {
		_ = os.Chdir(previous)
	}()
	return runCLIExpectSuccess(args, false)
}

func resetDir(root string) error {
	if err := os.RemoveAll(root); err != nil {
		return fmt.Errorf("remove %s: %w", root, err)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", root, err)
	}
	return nil
}

func writeFile(root, relPath, content string) error {
	path := filepath.Join(root, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir parent for %q: %w", relPath, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write %q: %w", relPath, err)
	}
	return nil
}

var fixedSnapshotTime = time.Unix(1700000000, 0)

type fileSnapshot struct {
	Contents string
	Mode     os.FileMode
	ModTime  int64
}

func snapshotFiles(root string) (map[string]fileSnapshot, error) {
	out := map[string]fileSnapshot{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = fileSnapshot{
			Contents: string(data),
			Mode:     info.Mode().Perm(),
			ModTime:  info.ModTime().UnixNano(),
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func setAllFileTimes(root string, ts time.Time) error {
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		return os.Chtimes(path, ts, ts)
	})
}

func diffSnapshots(first, second map[string]fileSnapshot) string {
	keys := map[string]bool{}
	for key := range first {
		keys[key] = true
	}
	for key := range second {
		keys[key] = true
	}
	ordered := make([]string, 0, len(keys))
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)
	for _, key := range ordered {
		a, aOK := first[key]
		b, bOK := second[key]
		switch {
		case !aOK:
			return fmt.Sprintf("file created on second run: %s", key)
		case !bOK:
			return fmt.Sprintf("file removed on second run: %s", key)
		case a != b:
			return fmt.Sprintf("file changed on second run: %s\nfirst: %#v\nsecond: %#v", key, a, b)
		}
	}
	return "snapshots differ"
}

func shrinkIdempotencyCase(c idempotencyCase, check func(idempotencyCase) string) idempotencyCase {
	for {
		changed := false
		for i := range c.Notes {
			candidate := c
			candidate.Notes = append(append([]propertyNote{}, c.Notes[:i]...), c.Notes[i+1:]...)
			if check(candidate) != "" {
				c = candidate
				changed = true
				break
			}
		}
		if changed {
			continue
		}
		for i, note := range c.Notes {
			simple := note.simplified()
			if reflect.DeepEqual(simple, note) {
				continue
			}
			candidate := c
			candidate.Notes = append([]propertyNote{}, c.Notes...)
			candidate.Notes[i] = simple
			if check(candidate) != "" {
				c = candidate
				changed = true
				break
			}
		}
		if changed {
			continue
		}
		fixtureShrinks := []func(idempotencyCase) idempotencyCase{
			func(v idempotencyCase) idempotencyCase { v.HasAgents = false; return v },
			func(v idempotencyCase) idempotencyCase { v.HasClaude = false; return v },
			func(v idempotencyCase) idempotencyCase { v.HasGitignore = false; return v },
			func(v idempotencyCase) idempotencyCase { v.HasHook = false; return v },
			func(v idempotencyCase) idempotencyCase { v.HasIgnore = false; return v },
			func(v idempotencyCase) idempotencyCase { v.HasGuide = false; return v },
		}
		for _, shrink := range fixtureShrinks {
			candidate := shrink(c)
			if reflect.DeepEqual(candidate, c) {
				continue
			}
			if check(candidate) != "" {
				c = candidate
				changed = true
				break
			}
		}
		if !changed {
			return c
		}
	}
}

func (n propertyNote) simplified() propertyNote {
	return propertyNote{
		Path:    n.Path,
		Title:   "A",
		Summary: "Summary.",
		Mode:    "append-only",
	}
}

func formatIdempotencyCase(c idempotencyCase) string {
	var b strings.Builder
	for i, note := range c.Notes {
		fmt.Fprintf(&b, "note %d: path=%q frontmatter=%t title=%q tags=%v mode=%q orient=%t updated=%q headings=%v link=%q\n", i, note.Path, note.Frontmatter, note.Title, note.Tags, note.Mode, note.Orient, note.Updated, note.Headings, note.LinkTarget)
	}
	fmt.Fprintf(&b, "fixtures: agents=%t claude=%t gitignore=%t hook=%t hookExecutable=%t ignore=%t guide=%t\n", c.HasAgents, c.HasClaude, c.HasGitignore, c.HasHook, c.HookExecutable, c.HasIgnore, c.HasGuide)
	return b.String()
}
