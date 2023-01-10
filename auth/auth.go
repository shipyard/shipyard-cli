package auth

import (
	"errors"

	"github.com/spf13/viper"
)

// GetAPIToken tries to read a token for the Shipyard API
// from the environment variable or loaded config (in that order).
func GetAPIToken() (string, error) {
	token := viper.GetString("SHIPYARD_API_TOKEN")
	if token == "" {
		return "", errors.New("token is missing, set the SHIPYARD_API_TOKEN environment/config value")
	}
	return token, nil
}
