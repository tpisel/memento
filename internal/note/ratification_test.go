package note

import (
	"testing"
)

func TestBindingForKeyTreatsTrackedPathspecMetacharactersLiterally(t *testing.T) {
	root := makeVault(t)
	initGit(t, root)
	key := "foo[bar].md"
	writeFile(t, root, key, "# Note\n\nTracked.\n")
	commitAll(t, root)

	got, err := BindingForKey(vaultFromRoot(root), key)
	if err != nil {
		t.Fatalf("BindingForKey(%q) error = %v, want nil", key, err)
	}
	if got != BindingRatified {
		t.Fatalf("BindingForKey(%q) = %s, want %s", key, got, BindingRatified)
	}
}

func TestBindingForKeyDoesNotRatifyUntrackedPathspecMetacharacterMatch(t *testing.T) {
	root := makeVault(t)
	initGit(t, root)
	writeFile(t, root, "foobar.md", "# Note\n\nTracked.\n")
	commitAll(t, root)

	key := "foo*.md"
	writeFile(t, root, key, "# Note\n\nUntracked.\n")

	got, err := BindingForKey(vaultFromRoot(root), key)
	if err != nil {
		t.Fatalf("BindingForKey(%q) error = %v, want nil", key, err)
	}
	if got != BindingUnratified {
		t.Fatalf("BindingForKey(%q) = %s, want %s", key, got, BindingUnratified)
	}
}
