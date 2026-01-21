package main

import (
	"github.com/caarlos0/env/v11"
)

// Config holds configuration for the PyPI patcher
type Config struct {
	APIKey     string `env:"ROOTIO_API_KEY,required"`
	APIURL     string `env:"ROOTIO_API_URL"          envDefault:"https://api.root.io"`
	PKGURL     string `env:"ROOTIO_PKG_URL"          envDefault:"https://pkg.root.io"`
	PythonPath string `env:"PYTHON_PATH"             envDefault:"python"`
	DryRun     bool   `env:"DRY_RUN"                 envDefault:"true"`
	UseAlias   bool   `env:"USE_ALIAS"               envDefault:"true"`
	LogLevel   string `env:"LOG_LEVEL"               envDefault:"info"`
}

// LoadConfig loads configuration from environment variables using caarlos0/env
func LoadConfig() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}
