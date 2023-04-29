package client

import (
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

	body, err := c.Requester.Do(http.MethodGet, uri.CreateResourceURI("", "environment", id, "", params), nil)
	if err != nil {
		return nil, err
	}

	return types.UnmarshalEnv(body)
}
