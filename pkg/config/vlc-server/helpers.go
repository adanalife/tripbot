package config

import "os"

func IsProduction() bool {
	return Conf.Environment == "production"
}

func IsStaging() bool {
	return Conf.Environment == "staging"
}

func IsDevelopment() bool {
	return Conf.Environment == "development"
}

func IsTesting() bool {
	return Conf.Environment == "testing"
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
