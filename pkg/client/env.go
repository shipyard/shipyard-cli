package client

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/shipyard/shipyard-cli/pkg/requests/uri"
	"github.com/shipyard/shipyard-cli/pkg/types"
)

// EnvByID tries to fetch an environment given its ID.
func (c Client) EnvByID(id string) (*types.Response, error) {
	if id == "" {
		return nil, errors.New("environment ID is an empty string")
	}

	params := make(map[string]string)
	if c.Org != "" {
		params["Org"] = c.Org
	}

	body, err := c.Requester.Do(http.MethodGet, uri.CreateResourceURI("", "environment", id, "", params), "application/json", nil)
	if err != nil {
		return nil, err
	}

	return types.UnmarshalEnv(body)
}

// AllEnvironmentUUIDs tries to fetch all environment by UUIDs in an org.
func (c Client) AllEnvironmentUUIDs() (*types.UUIDResponse, error) {
	params := make(map[string]string)
	if c.Org != "" {
		params["Org"] = c.Org
	}

	body, err := c.Requester.Do(http.MethodGet, uri.CreateResourceURI("", "environment/uuid", "", "", params), "application/json", nil)
	if err != nil {
		return nil, err
	}

	var res types.UUIDResponse
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, err
	}
	return &res, nil
}
