package auth

import (
	"errors"

	"github.com/spf13/viper"
)

func GetAPIToken() (string, error) {
	token := viper.GetString("SHIPYARD_API_TOKEN")
	if token == "" {
		return "", errors.New("token is missing, set the SHIPYARD_API_TOKEN config/environment variable")
	}
	return token, nil
}
