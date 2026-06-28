package enforce

import "testing"

func TestIsRatifiedTreatsNonGitTreeAsRatified(t *testing.T) {
	v := vaultFromRoot(makeVault(t))

	got, err := IsRatified(v, "notes/n.md")
	if err != nil {
		t.Fatalf("IsRatified error = %v, want nil", err)
	}
	if !got {
		t.Fatalf("IsRatified = false, want true for a non-git tree")
	}
}

func TestIsRatifiedTrueForCommittedFile(t *testing.T) {
	root := makeVault(t)
	v := vaultFromRoot(root)
	initGit(t, root)
	writeFile(t, root, "notes/tracked.md", "# Tracked\n")
	commitAll(t, root)

	got, err := IsRatified(v, "notes/tracked.md")
	if err != nil {
		t.Fatalf("IsRatified error = %v, want nil", err)
	}
	if !got {
		t.Fatalf("IsRatified = false, want true for a committed file")
	}
}

func TestIsRatifiedFalseForUntrackedFile(t *testing.T) {
	root := makeVault(t)
	v := vaultFromRoot(root)
	initGit(t, root)
	writeFile(t, root, "notes/untracked.md", "# Untracked\n")

	got, err := IsRatified(v, "notes/untracked.md")
	if err != nil {
		t.Fatalf("IsRatified error = %v, want nil", err)
	}
	if got {
		t.Fatalf("IsRatified = true, want false for an untracked file in a git tree")
	}
}
