package vault

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	MarkerDirName    = ".memento"
	ManifestFileName = "manifest.json"
)

var (
	ErrVaultNotFound  = errors.New("memento vault not found")
	ErrMultipleVaults = errors.New("multiple memento vaults found")
)

type Vault struct {
	Root         string
	MarkerDir    string
	ManifestPath string
}

type DiscoveryError struct {
	Err      error
	RepoRoot string
	Dir      string
	Markers  []string
}

func (e *DiscoveryError) Error() string {
	switch e.Err {
	case ErrVaultNotFound:
		if e.Dir != "" {
			return fmt.Sprintf("%v: expected %s marker directory in %s; run 'memento init' or pass --dir to an existing vault", e.Err, MarkerDirName, e.Dir)
		}
		return fmt.Sprintf("%v: expected exactly one %s marker directory under %s; run 'memento init' or pass --dir to an existing vault", e.Err, MarkerDirName, e.RepoRoot)
	case ErrMultipleVaults:
		return fmt.Sprintf("%v under %s: %s; remove extra %s markers or pass --dir to the intended vault", e.Err, e.RepoRoot, strings.Join(e.Markers, ", "), MarkerDirName)
	default:
		return e.Err.Error()
	}
}

func (e *DiscoveryError) Unwrap() error {
	return e.Err
}

func Discover(repoRoot string) (Vault, error) {
	root, err := filepath.Abs(repoRoot)
	if err != nil {
		return Vault{}, fmt.Errorf("resolve repository root %q: %w", repoRoot, err)
	}
	root = filepath.Clean(root)

	var markers []string
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		switch d.Name() {
		case ".git":
			return filepath.SkipDir
		case MarkerDirName:
			markers = append(markers, path)
			return filepath.SkipDir
		default:
			return nil
		}
	})
	if err != nil {
		return Vault{}, fmt.Errorf("discover memento vault under %s: %w", root, err)
	}

	switch len(markers) {
	case 0:
		return Vault{}, &DiscoveryError{Err: ErrVaultNotFound, RepoRoot: root}
	case 1:
		return vaultFromMarker(markers[0]), nil
	default:
		return Vault{}, &DiscoveryError{Err: ErrMultipleVaults, RepoRoot: root, Markers: markers}
	}
}

func Open(dir string) (Vault, error) {
	root, err := filepath.Abs(dir)
	if err != nil {
		return Vault{}, fmt.Errorf("resolve vault directory %q: %w", dir, err)
	}
	root = filepath.Clean(root)

	marker := filepath.Join(root, MarkerDirName)
	info, err := os.Stat(marker)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Vault{}, &DiscoveryError{Err: ErrVaultNotFound, Dir: root}
		}
		return Vault{}, fmt.Errorf("stat memento marker %s: %w", marker, err)
	}
	if !info.IsDir() {
		return Vault{}, &DiscoveryError{Err: ErrVaultNotFound, Dir: root}
	}
	return vaultFromMarker(marker), nil
}

func vaultFromMarker(marker string) Vault {
	return Vault{
		Root:         filepath.Dir(marker),
		MarkerDir:    marker,
		ManifestPath: filepath.Join(marker, ManifestFileName),
	}
}
