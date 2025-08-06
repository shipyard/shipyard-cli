package uri_test

import (
	"os"
	"testing"

	"github.com/shipyard/shipyard-cli/pkg/requests/uri"
)

func TestMain(m *testing.M) {
	// Set a consistent base URL for all tests
	_ = os.Setenv("SHIPYARD_BUILD_URL", "http://localhost:8080/api/v1")
	code := m.Run()
	os.Exit(code)
}

func TestCreateResourceURI(t *testing.T) {
	testCases := []struct {
		action      string
		resource    string
		id          string
		subresource string
		params      map[string]string

		name string
		want string
	}{
		{
			name:   "Get all environments",
			action: "", resource: "environment", id: "", subresource: "", params: nil,
			want: "http://localhost:8080/api/v1/environment",
		},
		{
			name:   "Get all environments in a specific org",
			action: "", resource: "environment", id: "", subresource: "", params: map[string]string{"org": "myorg"},
			want: "http://localhost:8080/api/v1/environment?org=myorg",
		},
		{
			name:   "Get all environments with filters applied",
			action: "", resource: "environment", id: "", subresource: "",
			params: map[string]string{"branch": "newfeature", "repo_name": "shipyard", "page_size": "9"},
			want:   "http://localhost:8080/api/v1/environment?branch=newfeature&page_size=9&repo_name=shipyard",
		},
		{
			name:   "Get a single environment",
			action: "", resource: "environment", id: "123abc", subresource: "",
			want: "http://localhost:8080/api/v1/environment/123abc",
		},
		{
			name:   "Get a single environment in a specific org",
			action: "", resource: "environment", id: "123abc", subresource: "", params: map[string]string{"org": "myorg"},
			want: "http://localhost:8080/api/v1/environment/123abc?org=myorg",
		},
		{
			name:   "Get a kubeconfig for a single environment",
			action: "", resource: "environment", id: "123abc", subresource: "kubeconfig", params: nil,
			want: "http://localhost:8080/api/v1/environment/123abc/kubeconfig",
		},
		{
			name:   "Get a kubeconfig for a single environment in a specific org",
			action: "", resource: "environment", id: "123abc", subresource: "kubeconfig", params: map[string]string{"org": "myorg"},
			want: "http://localhost:8080/api/v1/environment/123abc/kubeconfig?org=myorg",
		},
		{
			name:   "Stop a single environment",
			action: "stop", resource: "environment", id: "123abc", subresource: "", params: nil,
			want: "http://localhost:8080/api/v1/environment/123abc/stop",
		},
		{
			name:   "Stop a single environment in a specific org",
			action: "stop", resource: "environment", id: "123abc", subresource: "", params: map[string]string{"org": "myorg"},
			want: "http://localhost:8080/api/v1/environment/123abc/stop?org=myorg",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := uri.CreateResourceURI(tc.action, tc.resource, tc.id, tc.subresource, tc.params)
			if got != tc.want {
				t.Errorf("expected %s, but got %s", tc.want, got)
			}
		})
	}
}

func TestCreateResourceURIWithCustomBase(t *testing.T) {
	// Test with the base URL set in TestMain
	want := "http://localhost:8080/api/v1/environment/123abc"
	got := uri.CreateResourceURI("", "environment", "123abc", "", nil)
	if got != want {
		t.Errorf("expected %s, but got %s", want, got)
	}
}
