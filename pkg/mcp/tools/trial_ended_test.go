package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/shipyard/shipyard-cli/pkg/client"
)

type executor interface {
	Execute(ctx context.Context, params json.RawMessage) (string, error)
}

// Core refuses a change an expired trial couldn't build. The agent needs next_step
// and its url to tell the user what to do, not just the error's title.
func TestRefusalWithNextStepReachesTheAgent(t *testing.T) {
	refusal := `{"errors":[{"title":"This organization's trial has ended.","status":409}],"status":"needs_action",` +
		`"next_step":{"kind":"trial_ended","message":"This organization's trial has ended.","url":"https://example.com"}}`
	environmentArgs := `{"environment_id":"app-uuid-1"}`

	tests := []struct {
		name string
		tool func(c client.Client) executor
		args string
	}{
		{"rebuild_environment", func(c client.Client) executor { return NewEnvironmentTool(c, "rebuild_environment") }, environmentArgs},
		{"restart_environment", func(c client.Client) executor { return NewEnvironmentTool(c, "restart_environment") }, environmentArgs},
		{"revive_environment", func(c client.Client) executor { return NewEnvironmentTool(c, "revive_environment") }, environmentArgs},
		{"deploy_detached", func(c client.Client) executor { return NewExtendedTool(c, "deploy_detached") }, `{"application_build_id":"build-uuid-1"}`},
		{
			"update_branches",
			func(c client.Client) executor { return NewExtendedTool(c, "update_branches") },
			`{"environment_id":"app-uuid-1","projects":[{"repo_name":"app","branch":"main"}]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := newAppServer(t, http.StatusConflict, refusal)

			out, err := tt.tool(c).Execute(context.Background(), json.RawMessage(tt.args))
			if err != nil {
				t.Fatalf("a refusal is an answer, not a tool failure: %v", err)
			}
			for _, want := range []string{`"kind":"trial_ended"`, `"url":"https://example.com"`} {
				if !strings.Contains(out, want) {
					t.Errorf("result lost %s: %s", want, out)
				}
			}
		})
	}
}

func TestRefusalWithoutNextStepKeepsTheErrorText(t *testing.T) {
	c, _ := newAppServer(t, http.StatusBadRequest, `{"errors":[{"title":"This environment is not active"}]}`)

	out, err := NewEnvironmentTool(c, "rebuild_environment").Execute(context.Background(), json.RawMessage(`{"environment_id":"app-uuid-1"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "Cannot rebuild environment") {
		t.Errorf("got %q", out)
	}
}
