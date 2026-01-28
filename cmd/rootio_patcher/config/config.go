package config

import (
	"github.com/caarlos0/env/v11"
)

// Config holds configuration loaded from environment variables
type Config struct {
	APIKey   string `env:"ROOTIO_API_KEY,required"`
	APIURL   string `env:"ROOTIO_API_URL" envDefault:"https://api.root.io"`
	PKGURL   string `env:"ROOTIO_PKG_URL" envDefault:"https://pkg.root.io"`
	LogLevel string `env:"LOG_LEVEL" envDefault:"info"`
}

// LoadConfig loads configuration from environment variables using caarlos0/env
func LoadConfig() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}
