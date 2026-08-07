package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A run dir that can't be created has to come back as an error rather than
// ending the process, so main owns the exit and this stays testable at all.
func TestLoad_UnusableRunDirReturnsError(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocker, nil, 0600); err != nil {
		t.Fatalf("seeding the blocking file: %v", err)
	}
	t.Setenv("ONSCREENS_SERVER_RUN_DIR", filepath.Join(blocker, "run"))

	cfg, err := Load()
	if err == nil {
		t.Fatal("Load() succeeded with an uncreatable run dir, want an error")
	}
	if cfg != nil {
		t.Errorf("Load() = %+v on error, want nil config", cfg)
	}
	if !strings.Contains(err.Error(), "run dir") {
		t.Errorf("err = %v, want it to name the run dir", err)
	}
}

// The happy path creates the run dir and hands back the config.
func TestLoad_CreatesRunDir(t *testing.T) {
	runDir := filepath.Join(t.TempDir(), "nested", "run")
	t.Setenv("ONSCREENS_SERVER_RUN_DIR", runDir)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() = %v, want success", err)
	}
	if cfg.RunDir != runDir {
		t.Errorf("RunDir = %q, want %q", cfg.RunDir, runDir)
	}
	if fi, err := os.Stat(runDir); err != nil || !fi.IsDir() {
		t.Errorf("run dir not created: stat err = %v", err)
	}
}
