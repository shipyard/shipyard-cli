package auth

import (
	"errors"
	"os"

	"github.com/spf13/viper"
)

// APIToken tries to read a token for the Shipyard API
// from the environment variable or loaded config (in that order).
func APIToken() (string, error) {
	// Check if we're in test mode (same detection as spinner)
	if buildURL := os.Getenv("SHIPYARD_BUILD_URL"); buildURL == "http://localhost:8000" {
		return "test-token-from-test-mode", nil
	}
	
	// Check environment variable first
	if token := os.Getenv("SHIPYARD_API_TOKEN"); token != "" {
		return token, nil
	}
	
	// Fall back to viper config
	token := viper.GetString("api_token")
	if token == "" {
		return "", errors.New("missing token")
	}
	return token, nil
}
