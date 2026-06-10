package vault

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverFindsSingleMarker(t *testing.T) {
	repo := t.TempDir()
	memoryRoot := mkdir(t, repo, "project-memory", MarkerDirName)

	got, err := Discover(repo)
	if err != nil {
		t.Fatalf("Discover() error = %v, want nil", err)
	}

	wantRoot := filepath.Dir(memoryRoot)
	if got.Root != wantRoot {
		t.Fatalf("Discover().Root = %q, want %q", got.Root, wantRoot)
	}
	if got.MarkerDir != memoryRoot {
		t.Fatalf("Discover().MarkerDir = %q, want %q", got.MarkerDir, memoryRoot)
	}
	if got.ManifestPath != filepath.Join(memoryRoot, ManifestFileName) {
		t.Fatalf("Discover().ManifestPath = %q, want marker manifest path", got.ManifestPath)
	}
}

func TestDiscoverFollowsMarkerAfterVaultRename(t *testing.T) {
	repo := t.TempDir()
	oldRoot := mkdir(t, repo, "memento-memory")
	mkdir(t, oldRoot, MarkerDirName)

	newRoot := filepath.Join(repo, "renamed-vault")
	if err := os.Rename(oldRoot, newRoot); err != nil {
		t.Fatalf("rename vault: %v", err)
	}

	got, err := Discover(repo)
	if err != nil {
		t.Fatalf("Discover() error = %v, want nil", err)
	}
	if got.Root != newRoot {
		t.Fatalf("Discover().Root = %q, want renamed vault root %q", got.Root, newRoot)
	}
}

func TestDiscoverRequiresMarker(t *testing.T) {
	repo := t.TempDir()
	mkdir(t, repo, "notes")

	_, err := Discover(repo)
	if !errors.Is(err, ErrVaultNotFound) {
		t.Fatalf("Discover() error = %v, want ErrVaultNotFound", err)
	}
	if !strings.Contains(err.Error(), ".memento") || !strings.Contains(err.Error(), "memento init") {
		t.Fatalf("Discover() error = %q, want clear missing-marker guidance", err.Error())
	}
}

func TestDiscoverRejectsMultipleMarkers(t *testing.T) {
	repo := t.TempDir()
	first := mkdir(t, repo, "alpha-memory", MarkerDirName)
	second := mkdir(t, repo, "beta-memory", MarkerDirName)

	_, err := Discover(repo)
	if !errors.Is(err, ErrMultipleVaults) {
		t.Fatalf("Discover() error = %v, want ErrMultipleVaults", err)
	}
	for _, want := range []string{first, second, "multiple", ".memento"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("Discover() error = %q, want it to contain %q", err.Error(), want)
		}
	}
}

func TestOpenExplicitDirRequiresMarker(t *testing.T) {
	repo := t.TempDir()
	memoryRoot := mkdir(t, repo, "custom-name")

	_, err := Open(memoryRoot)
	if !errors.Is(err, ErrVaultNotFound) {
		t.Fatalf("Open() error = %v, want ErrVaultNotFound", err)
	}

	marker := mkdir(t, memoryRoot, MarkerDirName)
	got, err := Open(memoryRoot)
	if err != nil {
		t.Fatalf("Open() error = %v, want nil", err)
	}
	if got.Root != memoryRoot || got.MarkerDir != marker {
		t.Fatalf("Open() = %+v, want root %q and marker %q", got, memoryRoot, marker)
	}
}

func TestOpenExplicitDirBypassesSiblingAmbiguity(t *testing.T) {
	repo := t.TempDir()
	targetMarker := mkdir(t, repo, "target-memory", MarkerDirName)
	mkdir(t, repo, "other-memory", MarkerDirName)

	got, err := Open(filepath.Dir(targetMarker))
	if err != nil {
		t.Fatalf("Open() error = %v, want nil", err)
	}
	if got.MarkerDir != targetMarker {
		t.Fatalf("Open().MarkerDir = %q, want %q", got.MarkerDir, targetMarker)
	}
}

func mkdir(t *testing.T, parts ...string) string {
	t.Helper()

	path := filepath.Join(parts...)
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %q: %v", path, err)
	}
	return path
}
