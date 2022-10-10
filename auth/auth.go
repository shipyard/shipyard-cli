package auth

import (
	"errors"
	"os"
)

func GetAPIToken() (string, error) {
	token := os.Getenv("SHIPYARD_API_TOKEN")
	if token == "" {
		return "", errors.New("token is missing, set the SHIPYARD_API_TOKEN environment variable")
	}
	return token, nil
}
