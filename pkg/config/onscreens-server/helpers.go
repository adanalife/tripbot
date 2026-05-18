package config

func (c OnscreensServerConfig) IsProduction() bool {
	return c.Environment == "production"
}

func (c OnscreensServerConfig) IsStaging() bool {
	return c.Environment == "staging"
}

func (c OnscreensServerConfig) IsDevelopment() bool {
	return c.Environment == "development"
}

func (c OnscreensServerConfig) IsTesting() bool {
	return c.Environment == "testing"
}
