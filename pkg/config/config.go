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
	Environment() string
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
}
