//go:build !windows

package selfupdate

import (
	"errors"
	"os"
	"testing"
	"time"
)

func TestAskTerminal(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	ask := AskTerminal(r)

	start := time.Now()
	if _, err := ask(50 * time.Millisecond); !errors.Is(err, ErrNoAnswer) {
		t.Fatalf("no input: got %v, want ErrNoAnswer", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Errorf("timeout took %v", time.Since(start))
	}

	// Nothing was left reading after the timeout: the next line is intact.
	_, _ = w.Write([]byte("S\n"))
	if got, err := ask(time.Second); err != nil || got != "s" {
		t.Errorf("got %q, %v; want \"s\"", got, err)
	}

	_ = w.Close()
	if got, err := ask(time.Second); err != nil || got != "n" {
		t.Errorf("end of input: got %q, %v; want \"n\"", got, err)
	}
}
