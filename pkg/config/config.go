package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config interface {
	//Env() string
	IsProduction() bool
}

// setEnvironment sets the Environment var from the CLI
func SetEnvironment() {
	var err error
	var env string

	envVar, ok := os.LookupEnv("ENV")
	if !ok {
		log.Fatalln("You must set ENV")
	}

	// standardize the ENV
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
}
