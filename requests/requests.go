package requests

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"shipyard/auth"
	"shipyard/logging"
)

type Client interface {
	Do(method string, uri string, body any) ([]byte, error)
	Write([]byte) error
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

	logging.LogIfVerbose("URI", uri)

	req.Header.Set("Content-Type", "application/json")
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
		return nil, errors.New(string(b))
	}

	return b, nil
}

func (c httpClient) Write(p []byte) error {
	_, err := fmt.Fprintf(c.w, string(p))
	return err
}
