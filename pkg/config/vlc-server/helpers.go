package config

import "os"

func (c VlcServerConfig) IsProduction() bool {
	return c.Environment == "production"
}

func (c VlcServerConfig) IsStaging() bool {
	return c.Environment == "staging"
}

func (c VlcServerConfig) IsDevelopment() bool {
	return c.Environment == "development"
}

func (c VlcServerConfig) IsTesting() bool {
	return c.Environment == "testing"
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
