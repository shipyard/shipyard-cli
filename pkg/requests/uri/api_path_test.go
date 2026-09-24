package uri_test

import (
	"strings"
	"testing"

	"github.com/shipyard/shipyard-cli/pkg/requests/uri"
	"github.com/spf13/viper"
)

func TestResolveAPIPath(t *testing.T) {
	viper.Reset()
	defer viper.Reset()

	cases := []struct {
		name    string
		path    string
		org     string
		apiURL  string
		want    string
		wantErr string
	}{
		{
			name: "default host list",
			path: "/api/v1/environment",
			want: "https://shipyard.build/api/v1/environment",
		},
		{
			name: "org query",
			path: "/api/v1/environment",
			org:  "acme",
			want: "https://shipyard.build/api/v1/environment?org=acme",
		},
		{
			name: "preserves existing org",
			path: "/api/v1/environment?org=other",
			org:  "acme",
			want: "https://shipyard.build/api/v1/environment?org=other",
		},
		{
			name:   "local mode strips api v1 suffix",
			path:   "/api/v2/environment",
			apiURL: "http://localhost:8080/api/v1",
			want:   "http://localhost:8080/api/v2/environment",
		},
		{
			name:    "rejects absolute url",
			path:    "https://evil.example/api/v1/environment",
			wantErr: "absolute URLs",
		},
		{
			name:    "rejects non api prefix",
			path:    "/api/application/foo/deploy",
			wantErr: "must start with /api/v1/ or /api/v2/",
		},
		{
			name:    "rejects dot-dot escape from api prefix",
			path:    "/api/v1/../application/foo",
			wantErr: "must not contain . or ..",
		},
		{
			name:    "rejects encoded dot-dot",
			path:    "/api/v1/%2e%2e/me",
			wantErr: "must not contain . or ..",
		},
		{
			name:    "rejects lookalike prefix",
			path:    "/api/v1foo/environment",
			wantErr: "must start with /api/v1/ or /api/v2/",
		},
		{
			name:    "rejects bare version root",
			path:    "/api/v1",
			wantErr: "must start with /api/v1/ or /api/v2/",
		},
		{
			name:    "empty path",
			path:    "",
			wantErr: "path is required",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			viper.Reset()
			if tc.apiURL != "" {
				viper.Set("api_url", tc.apiURL)
			}
			got, err := uri.ResolveAPIPath(tc.path, tc.org)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}
