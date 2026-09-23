package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/shipyard/shipyard-cli/pkg/client"
)

type recordingRequester struct {
	method string
	uri    string
	body   any
	resp   []byte
	err    error
}

func (r *recordingRequester) Do(method, uri, contentType string, body any) ([]byte, error) {
	r.method = method
	r.uri = uri
	r.body = body
	if r.err != nil {
		return nil, r.err
	}
	if r.resp != nil {
		return r.resp, nil
	}
	return []byte(`{"ok":true}`), nil
}

func TestExtendedTool_GetBuildHistory(t *testing.T) {
	rec := &recordingRequester{resp: []byte(`{"data":{"builds":[]}}`)}
	tool := NewExtendedTool(client.Client{Requester: rec, OrgLookupFn: func() string { return "acme" }}, "get_build_history")

	out, err := tool.Execute(context.Background(), json.RawMessage(`{"environment_id":"env-123","page":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "builds") {
		t.Fatalf("unexpected body: %s", out)
	}
	if rec.method != "GET" || !strings.Contains(rec.uri, "environment/env-123/build-history") {
		t.Fatalf("unexpected request %s %s", rec.method, rec.uri)
	}
}

func TestExtendedTool_PutEnvVars(t *testing.T) {
	rec := &recordingRequester{}
	tool := NewExtendedTool(client.Client{Requester: rec, OrgLookupFn: func() string { return "" }}, "put_env_vars")

	_, err := tool.Execute(context.Background(), json.RawMessage(`{
		"environment_id":"env-123",
		"env_vars":[{"name":"DEBUG","value":"true","hidden":false}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if rec.method != "PUT" || !strings.Contains(rec.uri, "env-vars") {
		t.Fatalf("unexpected request %s %s", rec.method, rec.uri)
	}
}

func TestExtendedTool_DeployDetached(t *testing.T) {
	rec := &recordingRequester{resp: []byte(`{"data":{"application_uuid":"app-1"}}`)}
	tool := NewExtendedTool(client.Client{Requester: rec, OrgLookupFn: func() string { return "acme" }}, "deploy_detached")

	out, err := tool.Execute(context.Background(), json.RawMessage(`{
		"application_build_id":"build-123",
		"display_name":"agent-copy",
		"build_on_commit":"never"
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "app-1") {
		t.Fatalf("unexpected body: %s", out)
	}
	if rec.method != "POST" || !strings.Contains(rec.uri, "application-build/build-123/detached-app-build") {
		t.Fatalf("unexpected request %s %s", rec.method, rec.uri)
	}
}

func TestExtendedTool_CreateApplication(t *testing.T) {
	rec := &recordingRequester{resp: []byte(`{"data":{"id":"app-new","type":"application"}}`)}
	tool := NewExtendedTool(client.Client{Requester: rec, OrgLookupFn: func() string { return "acme" }}, "create_application")

	out, err := tool.Execute(context.Background(), json.RawMessage(`{
		"application_name":"demo-app",
		"projects":[{
			"repo_owner":"sks",
			"repo_name":"python-utility-fastapi-boto3-aws",
			"branch":"main",
			"services":["web"],
			"compose_filename":"docker-compose.yml"
		}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "app-new") {
		t.Fatalf("unexpected body: %s", out)
	}
	if rec.method != "POST" || !strings.Contains(rec.uri, "/application") {
		t.Fatalf("unexpected request %s %s", rec.method, rec.uri)
	}
	payload, ok := rec.body.(map[string]any)
	if !ok {
		t.Fatalf("expected map body, got %T", rec.body)
	}
	if payload["application_name"] != "demo-app" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

func TestExtendedTool_UpdateApplication(t *testing.T) {
	rec := &recordingRequester{resp: []byte(`{"data":{"id":"app-1","type":"application"}}`)}
	tool := NewExtendedTool(client.Client{Requester: rec, OrgLookupFn: func() string { return "acme" }}, "update_application")

	out, err := tool.Execute(context.Background(), json.RawMessage(`{
		"application_id":"app-1",
		"projects":[{
			"repo_owner":"sks",
			"repo_name":"python-utility-fastapi-boto3-aws",
			"branch":"main",
			"services":["web"],
			"compose_filename":"docker-compose.yml"
		}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "app-1") {
		t.Fatalf("unexpected body: %s", out)
	}
	if rec.method != "PATCH" || !strings.Contains(rec.uri, "/application/app-1") {
		t.Fatalf("unexpected request %s %s", rec.method, rec.uri)
	}
	payload, ok := rec.body.(map[string]any)
	if !ok {
		t.Fatalf("expected map body, got %T", rec.body)
	}
	projects, ok := payload["projects"].([]map[string]any)
	if !ok || len(projects) != 1 {
		t.Fatalf("unexpected projects: %#v", payload["projects"])
	}
	if projects[0]["compose_filename"] != "docker-compose.yml" {
		t.Fatalf("unexpected project payload: %#v", projects[0])
	}
}

func TestExtendedTool_UpdateApplication_RequiresID(t *testing.T) {
	rec := &recordingRequester{}
	tool := NewExtendedTool(client.Client{Requester: rec, OrgLookupFn: func() string { return "" }}, "update_application")

	_, err := tool.Execute(context.Background(), json.RawMessage(`{
		"projects":[{"repo_owner":"sks","repo_name":"web","branch":"main","services":["web"]}]
	}`))
	if err == nil {
		t.Fatal("expected validation error for missing application_id")
	}
	if rec.method != "" {
		t.Fatalf("should not call API on validation failure, got %s", rec.method)
	}
}

func TestExtendedTool_UpdateBranches(t *testing.T) {
	rec := &recordingRequester{}
	tool := NewExtendedTool(client.Client{Requester: rec, OrgLookupFn: func() string { return "" }}, "update_branches")

	_, err := tool.Execute(context.Background(), json.RawMessage(`{
		"environment_id":"env-123",
		"projects":[{"repo_name":"web","branch":"feature"}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if rec.method != "PATCH" {
		t.Fatalf("expected PATCH, got %s", rec.method)
	}
}

func TestExtendedTool_RestartService(t *testing.T) {
	rec := &recordingRequester{}
	tool := NewExtendedTool(client.Client{Requester: rec, OrgLookupFn: func() string { return "" }}, "restart_service")

	_, err := tool.Execute(context.Background(), json.RawMessage(`{"environment_id":"env-123","service_name":"web"}`))
	if err != nil {
		t.Fatal(err)
	}
	if rec.method != "POST" || !strings.Contains(rec.uri, "service/web/restart") {
		t.Fatalf("unexpected request %s %s", rec.method, rec.uri)
	}
}

func TestExtendedTool_Definition(t *testing.T) {
	tool := NewExtendedTool(client.Client{}, "get_env_vars")
	def := tool.Definition()
	if def.Name != "get_env_vars" {
		t.Fatalf("unexpected name %s", def.Name)
	}
	if def.InputSchema == nil {
		t.Fatal("expected input schema")
	}
}
