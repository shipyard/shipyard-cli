package resources

import (
	"context"
	"strings"
	"testing"

	"github.com/shipyard/shipyard-cli/pkg/client"
)

// Mock client for testing
func newMockClient() client.Client {
	return client.Client{}
}

func TestNewLogsResource(t *testing.T) {
	t.Parallel()

	client := newMockClient()
	resource := NewLogsResource(client)

	if resource == nil {
		t.Fatal("Expected resource to be created, got nil")
	}

	// Can't compare client structs directly due to function fields
	// Just check that the resource was initialized properly

	if resource.logsService == nil {
		t.Error("Expected logs service to be initialized")
	}

	if resource.uriPattern == nil {
		t.Error("Expected URI pattern to be compiled")
	}
}

func TestLogsResource_Definition(t *testing.T) {
	t.Parallel()

	client := newMockClient()
	resource := NewLogsResource(client)

	def := resource.Definition()

	if def.URI == "" {
		t.Error("Expected URI to be set")
	}

	if def.Name == "" {
		t.Error("Expected Name to be set")
	}

	if def.Description == "" {
		t.Error("Expected Description to be set")
	}

	if def.MimeType != "text/plain" {
		t.Errorf("Expected MimeType to be 'text/plain', got %s", def.MimeType)
	}

	expectedURI := "logs://{environment_id}/{service_name}"
	if def.URI != expectedURI {
		t.Errorf("Expected URI %s, got %s", expectedURI, def.URI)
	}
}

func TestLogsResource_GetContent(t *testing.T) {
	t.Parallel()

	// Skip this test as it requires proper mocking of client
	t.Skip("Requires proper client mocking - skipping for now")
}

func TestLogsResource_GetContent_InvalidURI(t *testing.T) {
	t.Parallel()

	client := newMockClient()
	resource := NewLogsResource(client)

	ctx := context.Background()

	// Test various invalid URIs
	invalidURIs := []string{
		"logs://",
		"logs://env-123",
		"logs:///service",
		"http://env-123/service",
		"logs://env-123/service/extra",
	}

	for _, uri := range invalidURIs {
		_, _, err := resource.GetContent(ctx, uri)
		if err == nil {
			t.Errorf("Expected error for invalid URI %s, got nil", uri)
		}
		if !strings.Contains(err.Error(), "invalid logs URI format") {
			t.Errorf("Expected invalid URI format error for %s, got: %v", uri, err)
		}
	}
}

func TestLogsResource_GetContent_WithQueryParams(t *testing.T) {
	t.Parallel()

	// Skip this test as it requires proper mocking of client
	t.Skip("Requires proper client mocking - skipping for now")
}

func TestLogsResource_GetContent_WithFollowParam(t *testing.T) {
	t.Parallel()

	// Skip this test as it requires proper mocking of client
	t.Skip("Requires proper client mocking - skipping for now")
}

func TestLogsResource_GetContent_WithTailParam(t *testing.T) {
	t.Parallel()

	// Skip this test as it requires proper mocking of client
	t.Skip("Requires proper client mocking - skipping for now")
}

func TestLogsResource_GetContent_ServiceNotFound(t *testing.T) {
	t.Parallel()

	// Skip this test as it requires proper mocking of client
	t.Skip("Requires proper client mocking - skipping for now")
}

func TestLogsResource_IsAvailable(t *testing.T) {
	t.Parallel()

	// Skip this test as it requires proper mocking of client
	t.Skip("Requires proper client mocking - skipping for now")
}

func TestLogsResource_IsAvailable_InvalidURI(t *testing.T) {
	t.Parallel()

	client := newMockClient()
	resource := NewLogsResource(client)

	ctx := context.Background()

	// Test various invalid URIs
	invalidURIs := []string{
		"logs://",
		"logs://env-123",
		"logs:///service",
		"http://env-123/service",
		"logs://env-123/service/extra",
	}

	for _, uri := range invalidURIs {
		available := resource.IsAvailable(ctx, uri)
		if available {
			t.Errorf("Expected resource to not be available for invalid URI %s", uri)
		}
	}
}

func TestLogsResource_IsAvailable_ServiceNotFound(t *testing.T) {
	t.Parallel()

	// Skip this test as it requires proper mocking of client
	t.Skip("Requires proper client mocking - skipping for now")
}

func TestLogsResource_GetResourceTemplate(t *testing.T) {
	t.Parallel()

	client := newMockClient()
	resource := NewLogsResource(client)

	template := resource.GetResourceTemplate()

	if template.URITemplate == "" {
		t.Error("Expected URITemplate to be set")
	}

	if template.Name == "" {
		t.Error("Expected Name to be set")
	}

	if template.Description == "" {
		t.Error("Expected Description to be set")
	}

	if template.MimeType != "text/plain" {
		t.Errorf("Expected MimeType to be 'text/plain', got %s", template.MimeType)
	}

	expectedURITemplate := "logs://{environment_id}/{service_name}"
	if template.URITemplate != expectedURITemplate {
		t.Errorf("Expected URITemplate %s, got %s", expectedURITemplate, template.URITemplate)
	}
}

func TestParseQueryParams_Empty(t *testing.T) {
	t.Parallel()

	params := parseQueryParams("")

	if len(params) != 0 {
		t.Errorf("Expected empty params map, got %v", params)
	}
}

func TestParseQueryParams_SingleParam(t *testing.T) {
	t.Parallel()

	params := parseQueryParams("follow=true")

	expected := map[string]string{
		"follow": "true",
	}

	if len(params) != len(expected) {
		t.Errorf("Expected %d params, got %d", len(expected), len(params))
	}

	for key, expectedValue := range expected {
		if actualValue, exists := params[key]; !exists {
			t.Errorf("Expected param %s to exist", key)
		} else if actualValue != expectedValue {
			t.Errorf("Expected param %s to be %s, got %s", key, expectedValue, actualValue)
		}
	}
}

func TestParseQueryParams_MultipleParams(t *testing.T) {
	t.Parallel()

	params := parseQueryParams("follow=true&tail=100&format=json")

	expected := map[string]string{
		"follow": "true",
		"tail":   "100",
		"format": "json",
	}

	if len(params) != len(expected) {
		t.Errorf("Expected %d params, got %d", len(expected), len(params))
	}

	for key, expectedValue := range expected {
		if actualValue, exists := params[key]; !exists {
			t.Errorf("Expected param %s to exist", key)
		} else if actualValue != expectedValue {
			t.Errorf("Expected param %s to be %s, got %s", key, expectedValue, actualValue)
		}
	}
}

func TestParseQueryParams_InvalidFormat(t *testing.T) {
	t.Parallel()

	// Test params without values
	params := parseQueryParams("follow&tail=100&format")

	// Should only parse the valid key=value pair
	if len(params) != 1 {
		t.Errorf("Expected 1 param, got %d", len(params))
	}

	if params["tail"] != "100" {
		t.Errorf("Expected tail=100, got %v", params)
	}
}

func TestLogsResource_URIPatternMatching(t *testing.T) {
	t.Parallel()

	client := newMockClient()
	resource := NewLogsResource(client)

	// Test valid URIs that should match the pattern
	validURIs := []struct {
		uri         string
		expectedEnv string
		expectedSvc string
	}{
		{"logs://env-123/web-server", "env-123", "web-server"},
		{"logs://abc123/my-service", "abc123", "my-service"},
		{"logs://env-456/api", "env-456", "api"},
		{"logs://test/service?follow=true", "test", "service"},
		{"logs://env/svc?tail=100&follow=false", "env", "svc"},
	}

	for _, test := range validURIs {
		matches := resource.uriPattern.FindStringSubmatch(test.uri)
		if len(matches) < 3 {
			t.Errorf("Expected URI %s to match pattern, got %v", test.uri, matches)
			continue
		}

		if matches[1] != test.expectedEnv {
			t.Errorf("Expected environment %s, got %s for URI %s", test.expectedEnv, matches[1], test.uri)
		}

		if matches[2] != test.expectedSvc {
			t.Errorf("Expected service %s, got %s for URI %s", test.expectedSvc, matches[2], test.uri)
		}
	}

	// Test invalid URIs that should not match
	invalidURIs := []string{
		"logs://",
		"logs://env",
		"logs:///service",
		"http://env/service",
		"logs://env/service/extra",
	}

	for _, uri := range invalidURIs {
		matches := resource.uriPattern.FindStringSubmatch(uri)
		if len(matches) >= 3 {
			t.Errorf("Expected URI %s to not match pattern, but it did: %v", uri, matches)
		}
	}
}
