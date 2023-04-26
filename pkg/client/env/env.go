package env

import (
	"errors"
	"io"
	"net/http"

	"github.com/shipyard/shipyard-cli/pkg/requests"
	"github.com/shipyard/shipyard-cli/pkg/requests/uri"
	"github.com/shipyard/shipyard-cli/pkg/types"
)

// GetByID tries to fetch an environment given its ID.
func GetByID(id, org string) (*types.Response, error) {
	if id == "" {
		return nil, errors.New("environment ID is an empty string")
	}

	requester, err := requests.New(io.Discard)
	if err != nil {
		return nil, err
	}

	params := make(map[string]string)
	if org != "" {
		params["org"] = org
	}

	body, err := requester.Do(http.MethodGet, uri.CreateResourceURI("", "environment", id, "", params), nil)
	if err != nil {
		return nil, err
	}

	return types.UnmarshalEnv(body)
}
