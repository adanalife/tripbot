package config

import (
	"os"
	"path/filepath"
	"testing"
)

// resolveFromRepoRoot should anchor a repo-relative path at the nearest
// ancestor dir containing go.mod, regardless of how deep cwd is — this is what
// lets a package's test binary (running from its own dir) find the repo-root
// .env.testing.
func TestResolveFromRepoRootFindsGoMod(t *testing.T) {
	// EvalSymlinks because macOS resolves /tmp -> /private/tmp once we chdir,
	// and resolveFromRepoRoot derives its result from os.Getwd().
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "pkg", "config", "tripbot")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	chdir(t, nested)

	got := resolveFromRepoRoot(".env.testing")
	want := filepath.Join(root, ".env.testing")
	if got != want {
		t.Errorf("resolveFromRepoRoot(%q) = %q, want %q", ".env.testing", got, want)
	}
}

// When no go.mod exists above cwd (e.g. a deployed binary), the bare relative
// path is returned so godotenv.Load no-ops on the missing file as before.
func TestResolveFromRepoRootFallsBackWithoutGoMod(t *testing.T) {
	chdir(t, t.TempDir())

	if got := resolveFromRepoRoot(".env.production"); got != ".env.production" {
		t.Errorf("resolveFromRepoRoot fallback = %q, want bare %q", got, ".env.production")
	}
}

// chdir switches into dir for the duration of the test, restoring the original
// working directory afterward. t.TempDir()s used here have no go.mod ancestor
// outside themselves, so the resolver's walk terminates at the filesystem root.
func chdir(t *testing.T, dir string) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })
}
