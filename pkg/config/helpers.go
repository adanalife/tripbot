package config

import "os"

func IsProduction() bool {
	return Environment == "production"
}

func IsStaging() bool {
	return Environment == "staging"
}

func IsDevelopment() bool {
	return Environment == "development"
}

func IsTesting() bool {
	return Environment == "testing"
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
