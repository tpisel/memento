package enforce

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/tpisel/memento/internal/vault"
)

func makeVault(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, vault.MarkerDirName), 0o755); err != nil {
		t.Fatalf("mkdir marker: %v", err)
	}
	return root
}

func vaultFromRoot(root string) vault.Vault {
	marker := filepath.Join(root, vault.MarkerDirName)
	return vault.Vault{
		Root:         root,
		MarkerDir:    marker,
		ManifestPath: filepath.Join(marker, vault.ManifestFileName),
	}
}

func writeFile(t *testing.T, root, relPath, content string) {
	t.Helper()

	path := filepath.Join(root, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir parent for %q: %v", relPath, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %q: %v", relPath, err)
	}
}

func initGit(t *testing.T, root string) {
	t.Helper()

	runGit(t, root, "init")
}

func commitAll(t *testing.T, root string) {
	t.Helper()

	runGit(t, root, "add", ".")
	runGit(t, root,
		"-c", "user.name=Memento Test",
		"-c", "user.email=memento-test@example.invalid",
		"commit", "--no-gpg-sign", "-m", "initial",
	)
}

func runGit(t *testing.T, root string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, string(output))
	}
}
