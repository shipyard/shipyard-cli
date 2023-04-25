package types

import (
	"encoding/json"
	"errors"
	"fmt"
)

var errUnmarshalling = errors.New("failed to unmarshal a value")

func UnmarshalEnv(p []byte) (*Response, error) {
	var r Response
	err := json.Unmarshal(p, &r)
	if err != nil {
		return nil, errUnmarshalling
	}
	return &r, err
}

func UnmarshalManyEnvs(p []byte) (*RespManyEnvs, error) {
	var r RespManyEnvs
	err := json.Unmarshal(p, &r)
	if err != nil {
		return nil, errUnmarshalling
	}
	return &r, err
}

func UnmarshalOrgs(body []byte) (*OrgsResponse, error) {
	var resp OrgsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal orgs response: %w", err)
	}

	return &resp, nil
}

type Response struct {
	Data struct {
		Environment
	} `json:"data"`
}

type RespManyEnvs struct {
	Data []struct {
		Environment
	} `json:"data"`
}

type OrgsResponse struct {
	Data []struct {
		Attributes struct {
			Name string `json:"name"`
		} `json:"attributes"`
	} `json:"data"`
}
