package requests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"shipyard/auth"
)

type Client interface {
	Do(method string, uri string, body any) ([]byte, error)
	Write([]byte) error
}

type httpClient struct {
	w     io.Writer
	token string
}

func NewClient(w io.Writer) (Client, error) {
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

	return b, nil
}

func (c httpClient) Write(p []byte) error {
	_, err := fmt.Fprintf(c.w, string(p))
	return err
}
