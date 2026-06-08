package auth

import (
	"errors"
	"os"

	"github.com/spf13/viper"
)

// APIToken tries to read a token for the Shipyard API
// from the environment variable or loaded config (in that order).
func APIToken() (string, error) {
	if token := os.Getenv("SHIPYARD_API_TOKEN"); token != "" {
		return token, nil
	}

	token := viper.GetString("api_token")
	if token == "" {
		return "", errors.New("token is missing, set the 'SHIPYARD_API_TOKEN' environment variable or 'api_token' config value")
	}
	return token, nil
}
