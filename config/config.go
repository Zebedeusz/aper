package config

type Config struct {
	ApiKey string   `yaml:"apiKey"`
	ApiURL string   `yaml:"apiURL"`
	Chains []string `yaml:"chains"`
}
