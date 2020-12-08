package config

import "os"

func IsProduction() bool {
	return cfg.Environment == "production"
}

func IsStaging() bool {
	return cfg.Environment == "staging"
}

func IsDevelopment() bool {
	return cfg.Environment == "development"
}

func IsTesting() bool {
	return cfg.Environment == "testing"
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
