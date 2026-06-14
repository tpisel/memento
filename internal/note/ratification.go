package note

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tpisel/memento/internal/vault"
)

type BindingState string

const (
	BindingRatified   BindingState = "ratified"
	BindingUnratified BindingState = "unratified"
)

func BindingForReadTarget(v vault.Vault, target string) (BindingState, error) {
	key, _, err := parseReadTarget(target)
	if err != nil {
		return "", err
	}
	return BindingForKey(v, key)
}

func BindingForKey(v vault.Vault, key string) (BindingState, error) {
	ratified, err := isRatified(v, key)
	if err != nil {
		return "", err
	}
	if ratified {
		return BindingRatified, nil
	}
	return BindingUnratified, nil
}

func isRatified(v vault.Vault, key string) (bool, error) {
	key, err := normalizeKey(key)
	if err != nil {
		return false, err
	}

	inside, err := isInsideGitWorkTree(v.Root)
	if err != nil {
		return false, err
	}
	if !inside {
		return true, nil
	}

	cmd := exec.Command("git", "--literal-pathspecs", "ls-files", "--error-unmatch", "--", filepath.FromSlash(key))
	cmd.Dir = v.Root
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err = cmd.Run()
	if err == nil {
		return true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if isFatalGitFailure(exitErr, stderr.String()) {
			return false, fmt.Errorf("check git ratification for %s: %w: %s", key, err, cleanGitStderr(stderr.String()))
		}
		return false, nil
	}
	return false, fmt.Errorf("check git ratification for %s: %w", key, err)
}

func isInsideGitWorkTree(root string) (bool, error) {
	cmd := exec.Command("git", "--literal-pathspecs", "rev-parse", "--is-inside-work-tree")
	cmd.Dir = root
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err == nil {
		return strings.TrimSpace(string(output)) == "true", nil
	}
	if errors.Is(err, exec.ErrNotFound) {
		return false, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		stderrText := stderr.String()
		if isNotGitRepository(stderrText) {
			hasMarker, markerErr := hasGitMarkerInAncestry(root)
			if markerErr != nil {
				return false, markerErr
			}
			if !hasMarker {
				return false, nil
			}
		}
		return false, fmt.Errorf("check git work tree: %w: %s", err, cleanGitStderr(stderrText))
	}
	return false, fmt.Errorf("check git work tree: %w", err)
}

func isFatalGitFailure(err *exec.ExitError, stderr string) bool {
	return err.ExitCode() == 128 || strings.HasPrefix(strings.TrimSpace(stderr), "fatal:")
}

func isNotGitRepository(stderr string) bool {
	return strings.Contains(stderr, "not a git repository")
}

func hasGitMarkerInAncestry(root string) (bool, error) {
	dir, err := filepath.Abs(root)
	if err != nil {
		return false, fmt.Errorf("inspect git marker ancestry for %s: %w", root, err)
	}
	for {
		gitMarker := filepath.Join(dir, ".git")
		_, err := os.Stat(gitMarker)
		switch {
		case err == nil:
			return true, nil
		case errors.Is(err, os.ErrNotExist):
		default:
			return false, fmt.Errorf("inspect git marker %s: %w", gitMarker, err)
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return false, nil
		}
		dir = parent
	}
}

func cleanGitStderr(stderr string) string {
	return strings.TrimSpace(stderr)
}
