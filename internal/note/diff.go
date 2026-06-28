package note

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tpisel/memento/internal/vault"
)

// ChangedNotesVsHead lists vault-relative keys whose working-tree bytes differ
// from HEAD (modified, deleted, or staged-but-new tracked files). It is the
// ratification-boundary diff — the working tree against the last commit, the same
// view the git pre-commit hook sees. Untracked files are excluded by git, which
// is exactly the brand-new-note carve-out the mode audit needs. A non-git tree,
// or a git tree with no commit yet (unborn HEAD), returns nil: nothing is
// ratified, so there is nothing to audit. Keys use forward slashes and are
// relative to the vault root (git's --relative, anchored at v.Root).
func ChangedNotesVsHead(v vault.Vault) ([]string, error) {
	inside, err := isInsideGitWorkTree(v.Root)
	if err != nil {
		return nil, err
	}
	if !inside {
		return nil, nil
	}

	cmd := exec.Command("git", "--literal-pathspecs", "diff", "--relative", "--name-only", "HEAD")
	cmd.Dir = v.Root
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if isUnbornHead(stderr.String()) {
			return nil, nil
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, fmt.Errorf("diff working tree against HEAD: %w: %s", err, cleanGitStderr(stderr.String()))
		}
		return nil, fmt.Errorf("diff working tree against HEAD: %w", err)
	}

	var keys []string
	for _, line := range strings.Split(stdout.String(), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		keys = append(keys, filepath.ToSlash(line))
	}
	return keys, nil
}

// HeadBytes returns key's ratified (HEAD) bytes. ok is false when key does not
// exist at HEAD — a brand-new note, which the mode audit must not flag — or when
// the tree is non-git / has no commit yet. The `HEAD:./` prefix resolves key
// relative to v.Root, so a vault nested under the repo root still maps correctly.
func HeadBytes(v vault.Vault, key string) ([]byte, bool, error) {
	inside, err := isInsideGitWorkTree(v.Root)
	if err != nil {
		return nil, false, err
	}
	if !inside {
		return nil, false, nil
	}

	cmd := exec.Command("git", "--literal-pathspecs", "show", "HEAD:./"+filepath.ToSlash(key))
	cmd.Dir = v.Root
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// A path absent at HEAD (or an unborn HEAD) exits 128 with a "does not
		// exist"/"bad revision" fatal — both mean "not ratified", so report ok=false
		// rather than erroring the whole audit. Any other ExitError is a real fault.
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("read HEAD bytes for %s: %w", key, err)
	}
	return stdout.Bytes(), true, nil
}

// isUnbornHead reports whether a git failure is the no-commit-yet case, where
// HEAD names no revision. git phrases this as an ambiguous/unknown-revision fatal.
func isUnbornHead(stderr string) bool {
	return strings.Contains(stderr, "unknown revision") ||
		strings.Contains(stderr, "ambiguous argument 'HEAD'") ||
		strings.Contains(stderr, "does not have any commits yet")
}
