package k8s

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// followLogs runs a followed Logs call against a server that answers with
// handler, and returns what it printed. It fails if Logs does not return: the
// activity heartbeat runs until it does, so a stuck call keeps an environment
// alive for as long as the process lives.
func followLogs(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	clientSet, err := kubernetes.NewForConfig(&rest.Config{Host: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	s := &Service{clientSet: clientSet, namespace: "app-ns", pod: "app-pod", container: "app"}

	stdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = stdout })

	done := make(chan error, 1)
	go func() { done <- s.Logs(true, 10) }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Logs did not return after the stream ended")
	}

	_ = w.Close()
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// A followed stream is chunked, so its end arrives as a read of 0 bytes and
// io.EOF. A pod restart or redeploy ends it this way.
func TestLogsReturnsWhenFollowedStreamEnds(t *testing.T) {
	out := followLogs(t, func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "hello\n")
		w.(http.Flusher).Flush()
		time.Sleep(50 * time.Millisecond)
	})
	if out != "hello\n" {
		t.Errorf("expected the stream's output, got %q", out)
	}
}

// A response with a known size delivers its last bytes together with io.EOF.
func TestLogsPrintsBytesReadWithEOF(t *testing.T) {
	out := followLogs(t, func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "hello\n")
	})
	if out != "hello\n" {
		t.Errorf("expected the final bytes to be printed, got %q", out)
	}
}
