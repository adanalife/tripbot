package config

import (
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/joho/godotenv"
)

// Config is used so we can pass TripbotConfig OR VlcServerConfig
// into some functions that need it
type Config interface {
	IsProduction() bool
	IsStaging() bool
}

// SetEnvironment loads in the ENV vars from a dotenv file
func SetEnvironment() {
	var err error
	var env string

	envVar, ok := os.LookupEnv("ENV")
	if !ok {
		// Host-side `go test ./pkg/...` runs with ENV unset; default those to
		// testing so the repo-root .env.testing (located via resolveFromRepoRoot
		// below) loads instead of the absent .env.development — the same env
		// `task test` sets explicitly. testing.Testing() is true only in test
		// binaries, so `go run` / production are unaffected. Everything else
		// defaults to development.
		if testing.Testing() {
			envVar = "testing"
		} else {
			envVar = "development"
			slog.Warn("ENV not set, defaulting to development")
		}
		// envconfig.Process reads from the process env; the defaulted
		// value has to be visible to the required:"true" field on
		// TripbotConfig.Environment / VlcServerConfig.Environment.
		os.Setenv("ENV", envVar)
	}

	// standardize the ENV to the long name
	switch envVar {
	case "stage", "staging":
		env = "staging"
	case "prod", "production":
		env = "production"
	case "dev", "development":
		env = "development"
	case "test", "testing":
		env = "testing"
	default:
		log.Fatalf("Unknown ENV: %s", envVar)
	}

	// load ENV vars from .env file
	//
	// resolveFromRepoRoot lets bare `go test ./pkg/foo` find the repo-root
	// .env.testing: a package's test binary runs from its own dir, so a plain
	// cwd-relative Load can't see the file. Walking up to go.mod makes the
	// blessed `task test` paths and host-side `go test` behave identically.
	err = godotenv.Load(resolveFromRepoRoot(".env." + env))

	// In cluster contexts (staging/production) the .env file is not shipped —
	// env values come from envconfig instead — so the missing-file error is
	// expected and noise. Only surface it for local-dev workflows.
	if err != nil && (env == "development" || env == "testing") {
		slog.Warn("error loading .env file, continuing anyway", "err", err, "env", env)
	}

	// Also load the docker env file as a base layer; docker-compose layers
	// this in via `--env-file infra/docker/env.docker`, but host-side runs
	// (e.g. cmd/auth-bootstrap) don't go through compose. godotenv.Load
	// doesn't overwrite existing values, so shell-env and .env.<env> stay
	// authoritative. Silent no-op in containers without this file present
	// (e.g. the cluster pod).
	_ = godotenv.Load(resolveFromRepoRoot("infra/docker/env.docker"))
}

// resolveFromRepoRoot turns a repo-relative path into an absolute one anchored
// at the module root (the nearest ancestor dir containing go.mod), so dotenv
// files resolve regardless of the process's working directory. This is what
// makes host-side `go test ./pkg/...` — whose test binaries run from each
// package's own dir — load the same .env.testing that `task test` does.
//
// If no go.mod is found above cwd (e.g. a deployed binary in a container where
// the dotenv files don't exist anyway), the bare relative path is returned and
// godotenv.Load no-ops on the missing file, preserving the prior behavior.
func resolveFromRepoRoot(rel string) string {
	dir, err := os.Getwd()
	if err != nil {
		return rel
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return filepath.Join(dir, rel)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return rel // reached filesystem root without finding go.mod
		}
		dir = parent
	}
}
