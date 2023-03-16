package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Token   string `yaml:"api_token"`
	Org     string `yaml:"org"`
	Verbose bool   `yaml:"verbose"`
}

// CreateDefaultConfig tries to create a config.yaml file in the default
// location for configuration files, which is $HOME/.shipyard.
// If that directory does not exist, the function creates it.
// It also pre-populates the file with keys for Shipyard's configurable values.
func CreateDefaultConfig(homedir string) error {
	p := filepath.Join(homedir, ".shipyard", "config.yaml")

	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return fmt.Errorf("failed to create the .shipyard directory in $HOME: %v", err)
	}

	var cfg Config
	b, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}

	return os.WriteFile(p, b, 0o600)
}
