package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

type Profile struct {
	AuthToken string `yaml:"auth_token"`
}

type Config struct {
	Token    string             `yaml:"api_token"`
	Org      string             `yaml:"org"`
	Verbose  bool               `yaml:"verbose"`
	ApiURL   string             `yaml:"api_url"`
	Profiles map[string]Profile `yaml:"profiles"`
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

// Save writes values into the config file viper loaded, and nothing else.
//
// The global viper also holds environment variables (AutomaticEnv with the
// SHIPYARD prefix), flag bindings and defaults. Its WriteConfig serializes all
// of that, so SHIPYARD_API_TOKEN from an MCP client's env block would replace
// the user's own token on disk. Save instead reads the file into a fresh viper,
// sets only the given keys and writes that back. The global viper is updated
// too, so the running process sees the new values.
func Save(values map[string]any) error {
	path := viper.ConfigFileUsed()
	if path == "" {
		return fmt.Errorf("no config file in use; run 'shipyard config init' first")
	}

	file := viper.New()
	file.SetConfigFile(path)
	if err := file.ReadInConfig(); err != nil {
		return fmt.Errorf("failed to read config: %w", err)
	}

	for key, value := range values {
		file.Set(key, value)
		viper.Set(key, value)
	}

	if err := file.WriteConfig(); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}
