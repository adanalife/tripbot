package config

func (c VlcServerConfig) Environment() string {
	return c.environment
}

func (c VlcServerConfig) IsProduction() bool {
	return c.environment == "production"
}

func (c VlcServerConfig) IsStaging() bool {
	return c.environment == "staging"
}

func (c VlcServerConfig) IsDevelopment() bool {
	return c.environment == "development"
}

func (c VlcServerConfig) IsTesting() bool {
	return c.environment == "testing"
}
