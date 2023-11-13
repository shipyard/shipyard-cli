package client

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/spf13/viper"

	"github.com/shipyard/shipyard-cli/pkg/requests"
	"github.com/shipyard/shipyard-cli/pkg/types"
)

func TestEnvByID(t *testing.T) {
	t.Parallel()

	client, cleanup := setup()
	defer cleanup()

	e, err := client.EnvByID("123abc")
	if err != nil {
		t.Fatal(err)
	}

	want := "123abc"
	if got := e.Data.ID; got != want {
		t.Errorf("expected to get environment with ID %s, but got %s", want, got)
	}
}

func TestAllServices(t *testing.T) {
	t.Parallel()

	client, cleanup := setup()
	defer cleanup()

	svcs, err := client.AllServices("123abc")
	if err != nil {
		t.Fatal(err)
	}

	want := 3
	if got := len(svcs); got != want {
		t.Errorf("expected to get %d services, but got %d", want, got)
	}
}

func TestFindService(t *testing.T) {
	t.Parallel()

	client, cleanup := setup()
	defer cleanup()

	got, err := client.FindService("web", "123abc")
	if err != nil {
		t.Fatal(err)
	}

	want := &types.Service{
		Name:          "web",
		Ports:         []string{"8080"},
		SanitizedName: "web",
	}
	if !cmp.Equal(got, want) {
		t.Errorf("want %s", cmp.Diff(got, want))
	}
}

func setup() (client Client, cleanup func()) {
	handler := newMux()
	server := httptest.NewServer(handler)
	_ = os.Setenv("SHIPYARD_BUILD_URL", server.URL)
	viper.Set("API_TOKEN", "fake-token")
	c := New(requests.New(), "")
	return c, func() {
		defer server.Close()
	}
}

func newMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/environment/123abc", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
  "data": {
    "attributes": {
      "name": "flask-starter", 
      "projects": [
        {
          "branch": "master", 
          "commit_hash": null, 
          "pull_request_number": null, 
          "repo_name": "flask-starter", 
          "repo_owner": "maxsokolovsky"
        }
      ], 
      "ready": false, 
      "retired": true, 
      "services": [
		{
          "name": "web", 
          "ports": [
            "8080"
          ], 
          "sanitized_name": "web", 
          "url": ""
        }, 
        {
          "name": "flower", 
          "ports": [
            "8081"
          ], 
          "sanitized_name": "flower", 
          "url": ""
        }, 
        {
          "name": "worker", 
          "ports": [], 
          "sanitized_name": "worker", 
          "url": ""
        }
      ], 
      "since_last_visit": 180, 
      "stopped": true, 
      "url": "https://123abc.dev.maxsokolovsky.shipyard.host"
    }, 
    "id": "123abc", 
    "links": {
      "kubeconfig": "/api/v1/environment/123abc/kubeconfig", 
      "self": "/api/v1/environment/123abc"
    }, 
    "type": "application"
  }
}`))
	})
	return mux
}
