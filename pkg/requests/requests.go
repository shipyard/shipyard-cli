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

	"github.com/shipyard/shipyard-cli/pkg/types"
	"github.com/shipyard/shipyard-cli/version"
)

type Requester interface {
	Do(method string, uri string, body any) ([]byte, error)
}

type HTTPClient struct {
	token string
}

func New(token string) HTTPClient {
	return HTTPClient{token: token}
}

func (c HTTPClient) Do(method, uri string, body any) ([]byte, error) {
	start := time.Now()
	defer func() {
		log.Println("Network request took", time.Since(start))
	}()
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

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*20)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, uri, reqBody)
	if err != nil {
		return nil, fmt.Errorf("error creating API request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", fmt.Sprintf("%s-%s-%s-%s", "shipyard-cli", version.Version, runtime.GOOS, runtime.GOARCH))
	req.Header.Set("x-api-token", c.token)

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
		errString := types.ParseErrorResponse(b)
		if errString == "" {
			return nil, errors.New(string(b))
		}
		// Force the first character of the error string from the API to be lower-case.
		errString = strings.ToLower(errString[:1]) + errString[1:]
		return nil, errors.New(errString)
	}

	return b, nil
}
