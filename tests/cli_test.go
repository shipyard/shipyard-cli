package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/shipyard/shipyard-cli/pkg/types"
	"github.com/shipyard/shipyard-cli/tests/server"
)

func TestMain(m *testing.M) {
	cmd := exec.Command("go", "build", "-o", "shipyard", "..")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		fmt.Printf("Setup failure: %s", stderr.String())
		os.Exit(1)
	}
	
	// Start test server on port 18081
	srv := &http.Server{
		Addr:              ":18081",
		ReadHeaderTimeout: time.Second,
		Handler:           server.NewHandler(),
	}
	
	// Channel to signal server startup status
	serverReady := make(chan error, 1)
	
	go func() {
		// Signal that server is starting
		serverReady <- srv.ListenAndServe()
	}()
	
	// Wait a bit for server to start, then check if it failed
	time.Sleep(100 * time.Millisecond)
	select {
	case err := <-serverReady:
		// Server failed to start immediately
		fmt.Printf("Failed to start test server on port 18081: %v\n", err)
		os.Exit(1)
	default:
		// Server appears to be running
	}

	code := m.Run()
	if err := os.Remove("shipyard"); err != nil {
		fmt.Printf("Cleanup failure: %v", err)
	}
	os.Exit(code)
}

func TestGetAllEnvironments(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		args   []string
		ids    []string
		output string
	}{
		{
			name: "default org",
			args: []string{"get", "envs", "--json"},
			ids:  []string{"default-1", "default-2"},
		},
		{
			name: "non default org",
			args: []string{"get", "envs", "--org", "pugs", "--json"},
			ids:  []string{"pug-1", "pug-2"},
		},
		{
			name:   "non existent org",
			args:   []string{"get", "envs", "--org", "cats"},
			output: "Command error: user org not found\n",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			c := newCmd(test.args)
			if err := c.cmd.Run(); err != nil {
				if diff := cmp.Diff(c.stdErr.String(), test.output); diff != "" {
					t.Error(diff)
				}
				return
			}
			var resp types.RespManyEnvs
			if err := json.Unmarshal(c.stdOut.Bytes(), &resp); err != nil {
				t.Fatal(err)
			}
			var ids []string
			for i := range resp.Data {
				ids = append(ids, resp.Data[i].ID)
			}
			want := test.ids
			if !cmp.Equal(ids, want) {
				t.Error(cmp.Diff(ids, want))
			}
		})
	}
}

func TestGetEnvironmentByID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		args   []string
		id     string
		output string
	}{
		{
			name: "default org",
			args: []string{"get", "env", "default-1", "--json"},
			id:   "default-1",
		},
		{
			name: "non default org",
			args: []string{"get", "env", "pug-1", "--org", "pugs", "--json"},
			id:   "pug-1",
		},
		{
			name:   "non existent env",
			args:   []string{"get", "env", "sharpei-1", "--org", "pugs", "--json"},
			output: "Command error: environment not found\n",
		},
		{
			name:   "non existent org",
			args:   []string{"get", "env", "cat-1", "--org", "cats"},
			output: "Command error: user org not found\n",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			c := newCmd(test.args)
			if err := c.cmd.Run(); err != nil {
				if diff := cmp.Diff(c.stdErr.String(), test.output); diff != "" {
					t.Error(diff)
				}
				return
			}
			var resp types.Response
			if err := json.Unmarshal(c.stdOut.Bytes(), &resp); err != nil {
				t.Fatal(err)
			}
			want := test.id
			got := resp.Data.ID
			if !cmp.Equal(got, want) {
				t.Error(cmp.Diff(got, want))
			}
		})
	}
}

func TestRebuildEnvironment(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		args   []string
		output string
	}{
		{
			name:   "default org",
			args:   []string{"rebuild", "env", "default-1"},
			output: "Environment queued for a rebuild.\n",
		},
		{
			name:   "non default org",
			args:   []string{"rebuild", "env", "pug-1", "--org", "pugs"},
			output: "Environment queued for a rebuild.\n",
		},
		{
			name:   "non existent env",
			args:   []string{"rebuild", "env", "sharpei-1", "--org", "pugs"},
			output: "Command error: environment not found\n",
		},
		{
			name:   "non existent org",
			args:   []string{"rebuild", "env", "pug-1", "--org", "cats"},
			output: "Command error: user org not found\n",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			c := newCmd(test.args)
			err := c.cmd.Run()
			if err != nil {
				if diff := cmp.Diff(c.stdErr.String(), test.output); diff != "" {
					t.Error(diff)
				}
				return
			}
			if diff := cmp.Diff(c.stdOut.String(), test.output); diff != "" {
				t.Error(diff)
			}
		})
	}
}

// nolint:gosec // Bad arguments can't be passed in.
func newCmd(args []string) *cmdWrapper {
	c := cmdWrapper{
		args: args,
	}
	c.cmd = exec.Command("./shipyard", commandLine(c.args)...)
	c.cmd.Env = []string{"SHIPYARD_BUILD_URL=http://localhost:18081"}
	stderr, stdout := new(bytes.Buffer), new(bytes.Buffer)
	c.cmd.Stderr = stderr
	c.cmd.Stdout = stdout
	c.stdErr = stderr
	c.stdOut = stdout
	return &c
}

func commandLine(in []string) []string {
	args := []string{"--config", "config.yaml"}
	args = append(args, in...)
	return args
}

type cmdWrapper struct {
	cmd    *exec.Cmd
	args   []string
	stdErr *bytes.Buffer
	stdOut *bytes.Buffer
}
