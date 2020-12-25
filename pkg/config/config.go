package config

type Config interface {
	//Env() string
	IsProduction() bool
}
