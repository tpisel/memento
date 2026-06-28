package enforce

import (
	"os"
	"testing"
	"time"
)

func TestLoadGrantsMissingSidecarIsEmpty(t *testing.T) {
	v := vaultFromRoot(makeVault(t))

	grants, err := LoadGrants(v)
	if err != nil {
		t.Fatalf("LoadGrants() error = %v, want nil", err)
	}
	if grants == nil {
		t.Fatal("LoadGrants() = nil, want non-nil empty map")
	}
	if len(grants) != 0 {
		t.Fatalf("LoadGrants() = %v, want empty", grants)
	}
}

func TestAddGrantThenLoadAndLookup(t *testing.T) {
	v := vaultFromRoot(makeVault(t))
	when := time.Date(2026, 6, 27, 9, 0, 0, 0, time.UTC)

	if err := AddGrant(v, "spec.md", "fixing a typo in the accepted record", when); err != nil {
		t.Fatalf("AddGrant() error = %v, want nil", err)
	}

	grants, err := LoadGrants(v)
	if err != nil {
		t.Fatalf("LoadGrants() error = %v, want nil", err)
	}
	got, ok := grants["spec.md"]
	if !ok {
		t.Fatalf("LoadGrants() = %v, want a grant for spec.md", grants)
	}
	if got.Justification != "fixing a typo in the accepted record" {
		t.Fatalf("Justification = %q, want the supplied reason", got.Justification)
	}
	if !got.GrantedAt.Equal(when) {
		t.Fatalf("GrantedAt = %v, want %v", got.GrantedAt, when)
	}

	g, found, err := LookupGrant(v, "spec.md")
	if err != nil {
		t.Fatalf("LookupGrant() error = %v, want nil", err)
	}
	if !found {
		t.Fatal("LookupGrant() found = false, want true")
	}
	if g.Justification != got.Justification {
		t.Fatalf("LookupGrant justification = %q, want %q", g.Justification, got.Justification)
	}

	if _, found, err := LookupGrant(v, "other.md"); err != nil || found {
		t.Fatalf("LookupGrant(other.md) = (%v, %v), want (false, nil)", found, err)
	}
}

func TestAddGrantMergesAndRefreshes(t *testing.T) {
	v := vaultFromRoot(makeVault(t))
	t0 := time.Date(2026, 6, 27, 9, 0, 0, 0, time.UTC)

	if err := AddGrant(v, "a.md", "first", t0); err != nil {
		t.Fatalf("AddGrant(a) error = %v", err)
	}
	if err := AddGrant(v, "b.md", "second", t0); err != nil {
		t.Fatalf("AddGrant(b) error = %v", err)
	}
	// Re-granting a.md must refresh in place, not drop b.md.
	t1 := t0.Add(time.Hour)
	if err := AddGrant(v, "a.md", "refreshed", t1); err != nil {
		t.Fatalf("AddGrant(a refresh) error = %v", err)
	}

	grants, err := LoadGrants(v)
	if err != nil {
		t.Fatalf("LoadGrants() error = %v", err)
	}
	if len(grants) != 2 {
		t.Fatalf("LoadGrants() = %v, want two grants", grants)
	}
	if a := grants["a.md"]; a.Justification != "refreshed" || !a.GrantedAt.Equal(t1) {
		t.Fatalf("a.md = %+v, want refreshed at %v", a, t1)
	}
	if b := grants["b.md"]; b.Justification != "second" {
		t.Fatalf("b.md = %+v, want preserved", b)
	}
}

func TestClearGrantsRemovesSidecar(t *testing.T) {
	v := vaultFromRoot(makeVault(t))
	if err := AddGrant(v, "spec.md", "reason", time.Now()); err != nil {
		t.Fatalf("AddGrant() error = %v", err)
	}
	if _, err := os.Stat(GrantsPath(v)); err != nil {
		t.Fatalf("sidecar missing after AddGrant: %v", err)
	}

	if err := ClearGrants(v); err != nil {
		t.Fatalf("ClearGrants() error = %v, want nil", err)
	}
	if _, err := os.Stat(GrantsPath(v)); !os.IsNotExist(err) {
		t.Fatalf("sidecar present after ClearGrants, stat err = %v", err)
	}

	grants, err := LoadGrants(v)
	if err != nil || len(grants) != 0 {
		t.Fatalf("LoadGrants() after clear = (%v, %v), want empty", grants, err)
	}

	// Clearing again is a no-op, not an error.
	if err := ClearGrants(v); err != nil {
		t.Fatalf("ClearGrants() second call error = %v, want nil", err)
	}
}

func TestLoadGrantsMalformedIsError(t *testing.T) {
	v := vaultFromRoot(makeVault(t))
	if err := os.WriteFile(GrantsPath(v), []byte("{not json"), 0o644); err != nil {
		t.Fatalf("seed malformed sidecar: %v", err)
	}

	if _, err := LoadGrants(v); err == nil {
		t.Fatal("LoadGrants() error = nil, want a parse error for malformed sidecar")
	}
}
