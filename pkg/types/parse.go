package types

import (
	"encoding/json"
	"errors"
	"fmt"
)

var errUnmarshalling = errors.New("failed to unmarshal a value")

func UnmarshalEnv(p []byte) (*Response, error) {
	var r Response
	if err := json.Unmarshal(p, &r); err != nil {
		return nil, errUnmarshalling
	}
	return &r, nil
}

func UnmarshalManyEnvs(p []byte) (*RespManyEnvs, error) {
	var r RespManyEnvs
	if err := json.Unmarshal(p, &r); err != nil {
		return nil, errUnmarshalling
	}
	return &r, nil
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

func ParseErrorResponse(p []byte) string {
	var r errorResponse
	if err := json.Unmarshal(p, &r); err != nil {
		return ""
	}
	if len(r.Errors) == 0 || r.Errors[0].Title == "" {
		return ""
	}
	return r.Errors[0].Title
}

type errorResponse struct {
	Errors []struct {
		Status int    `json:"status"`
		Title  string `json:"title"`
	} `json:"errors"`
}