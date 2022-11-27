package requests

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"runtime"

	"shipyard/auth"
	"shipyard/version"
)

type Client interface {
	Do(method string, uri string, body any) ([]byte, error)
	Write(any) error
}

type httpClient struct {
	w     io.Writer
	token string
}

func NewHTTPClient(w io.Writer) (Client, error) {
	token, err := auth.GetAPIToken()
	if err != nil {
		return nil, err
	}
	return &httpClient{token: token, w: w}, nil
}

func (c httpClient) Do(method string, uri string, body any) ([]byte, error) {
	log.Println("URI", uri)

	var reqBody io.Reader
	if body == nil {
		reqBody = nil
	} else if d, ok := body.([]byte); ok {
		reqBody = bytes.NewReader(d)
	} else {
		serialized, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reqBody = bytes.NewReader(serialized)
	}

	req, err := http.NewRequest(method, uri, reqBody)
	if err != nil {
		return nil, fmt.Errorf("error creating API request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", fmt.Sprintf("%s-%s-%s-%s", "shipyard-cli", version.Version, runtime.GOOS, runtime.GOARCH))
	req.Header.Set("x-api-token", c.token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error sending API request: %w", err)
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if len(b) == 0 {
			return nil, fmt.Errorf("empty response")
		}
		parsedError := parseError(b)
		if parsedError == "" {
			return nil, errors.New(string(b))
		}
		return nil, errors.New(parsedError)
	}

	return b, nil
}

func (c httpClient) Write(data any) error {
	_, err := fmt.Fprintf(c.w, "%s", data)
	return err
}

func parseError(p []byte) string {
	var r errorResponse
	if err := json.Unmarshal(p, &r); err != nil {
		return ""
	}
	if len(r.Errors) == 0 {
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
