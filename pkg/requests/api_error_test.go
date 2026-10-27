package requests

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDoReturnsAPIError(t *testing.T) {
	t.Setenv("SHIPYARD_API_TOKEN", "fake-token")

	tests := []struct {
		name    string
		status  int
		body    string
		wantMsg string
		// wantStatus is the "status" field read back from the body, if any.
		wantStatus string
	}{
		{
			name:    "JSON:API error title is lower-cased",
			status:  http.StatusBadRequest,
			body:    `{"errors":[{"title":"Invalid repo or branch name.","status":400}]}`,
			wantMsg: "invalid repo or branch name.",
		},
		{
			name:    "body without an error title is the message",
			status:  http.StatusInternalServerError,
			body:    `{"error":"boom"}`,
			wantMsg: `{"error":"boom"}`,
		},
		{
			name:    "empty body",
			status:  http.StatusBadGateway,
			body:    "",
			wantMsg: "empty response",
		},
		{
			name:       "needs_action body is kept",
			status:     http.StatusConflict,
			body:       `{"errors":[{"title":"Install the GitHub app"}],"status":"needs_action","next_step":{"kind":"install_github_app","url":"https://example.com/install","message":"Install the GitHub app"}}`,
			wantMsg:    "install the GitHub app",
			wantStatus: "needs_action",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			_, err := New().Do(http.MethodPost, server.URL, "application/json", nil)
			if err == nil {
				t.Fatal("expected an error")
			}
			if err.Error() != tt.wantMsg {
				t.Errorf("message = %q, want %q", err.Error(), tt.wantMsg)
			}
			apiErr, ok := AsAPIError(fmt.Errorf("wrapped: %w", err))
			if !ok {
				t.Fatalf("expected an *APIError, got %T", err)
			}
			if apiErr.StatusCode != tt.status {
				t.Errorf("StatusCode = %d, want %d", apiErr.StatusCode, tt.status)
			}
			if string(apiErr.Body) != tt.body {
				t.Errorf("Body = %q, want %q", apiErr.Body, tt.body)
			}
			got, _ := apiErr.JSONObject()["status"].(string)
			if got != tt.wantStatus {
				t.Errorf("body status = %q, want %q", got, tt.wantStatus)
			}
		})
	}
}

func TestDoSuccessIsNotAnError(t *testing.T) {
	t.Setenv("SHIPYARD_API_TOKEN", "fake-token")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"data":{}}`))
	}))
	defer server.Close()

	body, err := New().Do(http.MethodPost, server.URL, "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `{"data":{}}` {
		t.Errorf("body = %s", body)
	}
}

func TestAsAPIErrorOtherErrors(t *testing.T) {
	if _, ok := AsAPIError(fmt.Errorf("timeout")); ok {
		t.Error("a plain error is not an APIError")
	}
	if _, ok := AsAPIError(nil); ok {
		t.Error("nil is not an APIError")
	}
}
