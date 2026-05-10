package config

import (
	"log"
	"os"

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
		log.Fatalln("You must set ENV")
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
	err = godotenv.Load(".env." + env)

	if err != nil {
		log.Println("Error loading .env file:", err)
		log.Println("Continuing anyway...")
	}

	// Also load the docker env file as a base layer; docker-compose layers
	// this in via `--env-file infra/docker/env.docker`, but host-side runs
	// (e.g. cmd/auth-bootstrap) don't go through compose. godotenv.Load
	// doesn't overwrite existing values, so shell-env and .env.<env> stay
	// authoritative. Silent no-op in containers without this file present
	// (e.g. the cluster pod).
	_ = godotenv.Load("infra/docker/env.docker")
}
