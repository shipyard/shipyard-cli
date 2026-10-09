package k8s

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/shipyard/shipyard-cli/pkg/client"
	"github.com/shipyard/shipyard-cli/pkg/types"
)

// kubeconfigRequester answers the kubeconfig request with a config for server.
type kubeconfigRequester struct{ server string }

func (r kubeconfigRequester) Do(_, _, _ string, _ any) ([]byte, error) {
	return []byte(fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- name: env
  cluster:
    server: %s
contexts:
- name: env
  context:
    cluster: env
    namespace: app-ns
current-context: env
users:
- name: env
`, r.server)), nil
}

// get_logs is labeled read-only for MCP clients, and logs, exec and
// port-forward only need the config in memory. New must not overwrite the
// shared ~/.shipyard/kubeconfig, which a running telepresence session for
// another environment may be using.
func TestNewDoesNotWriteTheSharedKubeconfig(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v1.PodList{Items: []v1.Pod{{ObjectMeta: metav1.ObjectMeta{Name: "web-0"}}}})
	}))
	t.Cleanup(api.Close)

	home := t.TempDir()
	t.Setenv("HOME", home)
	shared := filepath.Join(home, ".shipyard", "kubeconfig")
	if err := os.MkdirAll(filepath.Dir(shared), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(shared, []byte("another environment"), 0o600); err != nil {
		t.Fatal(err)
	}

	c := client.New(kubeconfigRequester{server: api.URL}, func() string { return "" })
	s, err := New(c, "env-id", &types.Service{Name: "web", SanitizedName: "web"})
	if err != nil {
		t.Fatal(err)
	}
	if s.pod != "web-0" || s.namespace != "app-ns" {
		t.Errorf("pod %q in namespace %q, want web-0 in app-ns", s.pod, s.namespace)
	}
	if got, _ := os.ReadFile(shared); string(got) != "another environment" {
		t.Errorf("New overwrote the shared kubeconfig with:\n%s", got)
	}
}
