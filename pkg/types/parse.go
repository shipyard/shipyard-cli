package types

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
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
	Data Environment `json:"data"`
}

type RespManyEnvs struct {
	Data  []Environment `json:"data"`
	Links Links         `json:"links"`
}

type UUIDResponse struct {
	Data []string `json:"data"`
}

type OrgsResponse struct {
	Data []struct {
		Attributes struct {
			Name string `json:"name"`
		} `json:"attributes"`
	} `json:"data"`
}

type VolumesResponse struct {
	Data []Volume `json:"data"`
}

type SnapshotsResponse struct {
	Data  []Snapshot `json:"data"`
	Links Links      `json:"links"`
}

type Links struct {
	First string `json:"first"`
	Last  string `json:"last"`
	Next  string `json:"next"`
	Prev  string `json:"prev"`
}

// NextPage extracts the value of the "page" query parameter of the "next" URL.
func (l Links) NextPage() int {
	parsed, err := url.Parse(l.Next)
	if err != nil {
		return 0
	}
	page := parsed.Query().Get("page")
	i, _ := strconv.Atoi(page)
	return i
}

func ErrorFromResponse(p []byte) string {
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
