package requests

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/shipyard/shipyard-cli/auth"
	"github.com/shipyard/shipyard-cli/pkg/types"
	"github.com/shipyard/shipyard-cli/version"
)

type Requester interface {
	Do(method string, uri string, contentType string, body any) ([]byte, error)
}

type HTTPClient struct {
}

func New() HTTPClient {
	return HTTPClient{}
}

func (c HTTPClient) Do(method, uri, contentType string, body any) ([]byte, error) {
	var token string
	var err error
	// TODO: refactor the CLI initialization process this to make the client not depend on global state.
	token, err = auth.APIToken()
	if err != nil {
		return nil, err
	}
	start := time.Now()
	defer func() {
		log.Println("Network request took", time.Since(start))
	}()
	log.Println("URI", uri)

	var reqBody io.Reader
	switch body := body.(type) {
	case nil:
		// For nil body (common with GET requests), use nil reader
		reqBody = nil
	case []byte:
		reqBody = bytes.NewReader(body)
	case *bytes.Buffer:
		reqBody = bytes.NewReader(body.Bytes())
	default:
		serialized, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reqBody = bytes.NewReader(serialized)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*20)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, uri, reqBody)
	if err != nil {
		return nil, fmt.Errorf("error creating API request: %w", err)
	}

	// Only set Content-Type if there's a body
	if reqBody != nil {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("User-Agent", fmt.Sprintf("%s-%s-%s-%s", "shipyard-cli", version.Version, runtime.GOOS, runtime.GOARCH))
	req.Header.Set("x-api-token", token)

	var netClient http.Client
	resp, err := netClient.Do(req)
	if err != nil {
		if os.IsTimeout(err) {
			return nil, fmt.Errorf("timeout - server took too long to respond")
		}
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
		errString := types.ErrorFromResponse(b)
		if errString == "" {
			return nil, errors.New(string(b))
		}
		// Force the first character of the error string from the API to be lower-case.
		errString = strings.ToLower(errString[:1]) + errString[1:]
		return nil, errors.New(errString)
	}

	return b, nil
}
