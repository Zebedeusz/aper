package config

type Config struct {
	ApiKey        string   `yaml:"apiKey"`
	MoralisApiKey string   `yaml:"moralisApiKey"`
	Chains        []string `yaml:"chains"`
}
