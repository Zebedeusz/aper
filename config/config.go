package config

type Config struct {
	ApiKey string   `yaml:"apiKey"`
	Chains []string `yaml:"chains"`
}
