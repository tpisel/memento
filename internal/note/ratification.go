package note

import (
	"errors"
	"fmt"
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
	err = cmd.Run()
	if err == nil {
		return true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return false, nil
	}
	return false, fmt.Errorf("check git ratification for %s: %w", key, err)
}

func isInsideGitWorkTree(root string) (bool, error) {
	cmd := exec.Command("git", "--literal-pathspecs", "rev-parse", "--is-inside-work-tree")
	cmd.Dir = root
	output, err := cmd.Output()
	if err == nil {
		return strings.TrimSpace(string(output)) == "true", nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) || errors.Is(err, exec.ErrNotFound) {
		return false, nil
	}
	return false, fmt.Errorf("check git work tree: %w", err)
}
