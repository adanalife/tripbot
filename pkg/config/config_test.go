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

	t.Chdir(nested)

	got := resolveFromRepoRoot(".env.testing")
	want := filepath.Join(root, ".env.testing")
	if got != want {
		t.Errorf("resolveFromRepoRoot(%q) = %q, want %q", ".env.testing", got, want)
	}
}

// When no go.mod exists above cwd (e.g. a deployed binary), the bare relative
// path is returned so godotenv.Load no-ops on the missing file as before.
func TestResolveFromRepoRootFallsBackWithoutGoMod(t *testing.T) {
	t.Chdir(t.TempDir())

	if got := resolveFromRepoRoot(".env.production"); got != ".env.production" {
		t.Errorf("resolveFromRepoRoot fallback = %q, want bare %q", got, ".env.production")
	}
}

// With ENV unset under `go test`, SetEnvironment should default to testing (so
// the repo-root .env.testing loads), not development — otherwise host-side
// `go test ./pkg/...` fails on the absent .env.development + missing required
// config keys.
func TestSetEnvironmentDefaultsToTestingUnderGoTest(t *testing.T) {
	// SetEnvironment branches on os.LookupEnv's ok, so this test needs ENV
	// genuinely absent — t.Setenv can only set it to a value (including ""),
	// which takes the other branch. Hence the manual save/restore.
	orig, had := os.LookupEnv("ENV")
	os.Unsetenv("ENV")
	t.Cleanup(func() {
		if had {
			//nolint:usetesting // t.Setenv is unusable from a Cleanup, and "" != unset
			os.Setenv("ENV", orig)
		} else {
			os.Unsetenv("ENV")
		}
	})

	SetEnvironment()

	if got := os.Getenv("ENV"); got != "testing" {
		t.Errorf("SetEnvironment() with ENV unset under go test: ENV=%q, want %q", got, "testing")
	}
}
