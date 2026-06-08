package validation

import (
	"strings"
	"testing"
)

func TestValidateEnvironmentID(t *testing.T) {
	tests := []struct {
		name      string
		id        string
		expectErr bool
	}{
		{"valid ID", "env-123", false},
		{"valid alphanumeric", "abc123", false},
		{"valid with underscores", "env_test_123", false},
		{"valid with hyphens", "env-test-123", false},
		{"empty ID", "", true},
		{"too short", "ab", true},
		{"too long", strings.Repeat("a", 101), true},
		{"invalid characters - space", "env 123", true},
		{"invalid characters - slash", "env/123", true},
		{"invalid characters - dots", "env.123", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateEnvironmentID(tt.id)
			if tt.expectErr && err == nil {
				t.Errorf("Expected error for ID %q, got nil", tt.id)
			}
			if !tt.expectErr && err != nil {
				t.Errorf("Expected no error for ID %q, got %v", tt.id, err)
			}
		})
	}
}

func TestValidateServiceName(t *testing.T) {
	tests := []struct {
		name      string
		service   string
		expectErr bool
	}{
		{"valid service", "web", false},
		{"valid with hyphens", "web-server", false},
		{"valid with underscores", "api_service", false},
		{"valid alphanumeric", "service123", false},
		{"empty service", "", true},
		{"too long", strings.Repeat("a", 101), true},
		{"invalid characters - space", "web server", true},
		{"invalid characters - slash", "web/server", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateServiceName(tt.service)
			if tt.expectErr && err == nil {
				t.Errorf("Expected error for service %q, got nil", tt.service)
			}
			if !tt.expectErr && err != nil {
				t.Errorf("Expected no error for service %q, got %v", tt.service, err)
			}
		})
	}
}

func TestValidateRepoName(t *testing.T) {
	tests := []struct {
		name      string
		repo      string
		expectErr bool
	}{
		{"empty repo (optional)", "", false},
		{"simple repo", "myrepo", false},
		{"org/repo format", "myorg/myrepo", false},
		{"with hyphens", "my-org/my-repo", false},
		{"with underscores", "my_org/my_repo", false},
		{"with dots", "my.org/my.repo", false},
		{"too long", strings.Repeat("a", 201), true},
		{"invalid format - multiple slashes", "org/repo/invalid", true},
		{"invalid characters - space", "my org/repo", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRepoName(tt.repo)
			if tt.expectErr && err == nil {
				t.Errorf("Expected error for repo %q, got nil", tt.repo)
			}
			if !tt.expectErr && err != nil {
				t.Errorf("Expected no error for repo %q, got %v", tt.repo, err)
			}
		})
	}
}

func TestValidateBranchName(t *testing.T) {
	tests := []struct {
		name      string
		branch    string
		expectErr bool
	}{
		{"empty branch (optional)", "", false},
		{"simple branch", "main", false},
		{"feature branch", "feature/new-feature", false},
		{"with hyphens", "feature-branch", false},
		{"with underscores", "feature_branch", false},
		{"too long", strings.Repeat("a", 251), true},
		{"starts with slash", "/invalid", true},
		{"ends with slash", "invalid/", true},
		{"consecutive dots", "feature..branch", true},
		{"invalid characters - space", "feature branch", true},
		{"invalid characters - colon", "feature:branch", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateBranchName(tt.branch)
			if tt.expectErr && err == nil {
				t.Errorf("Expected error for branch %q, got nil", tt.branch)
			}
			if !tt.expectErr && err != nil {
				t.Errorf("Expected no error for branch %q, got %v", tt.branch, err)
			}
		})
	}
}

func TestValidatePagination(t *testing.T) {
	tests := []struct {
		name      string
		page      int
		pageSize  int
		expectErr bool
	}{
		{"valid pagination", 1, 20, false},
		{"zero values (defaults)", 0, 0, false},
		{"negative page", -1, 20, true},
		{"negative page size", 1, -1, true},
		{"page too large", 10001, 20, true},
		{"page size too large", 1, 1001, true},
		{"max valid values", 10000, 1000, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePagination(tt.page, tt.pageSize)
			if tt.expectErr && err == nil {
				t.Errorf("Expected error for page=%d, pageSize=%d, got nil", tt.page, tt.pageSize)
			}
			if !tt.expectErr && err != nil {
				t.Errorf("Expected no error for page=%d, pageSize=%d, got %v", tt.page, tt.pageSize, err)
			}
		})
	}
}

func TestValidateLogTail(t *testing.T) {
	tests := []struct {
		name      string
		tail      int
		expectErr bool
	}{
		{"valid tail", 100, false},
		{"zero tail", 0, false},
		{"max tail", 10000, false},
		{"negative tail", -1, true},
		{"tail too large", 10001, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateLogTail(tt.tail)
			if tt.expectErr && err == nil {
				t.Errorf("Expected error for tail=%d, got nil", tt.tail)
			}
			if !tt.expectErr && err != nil {
				t.Errorf("Expected no error for tail=%d, got %v", tt.tail, err)
			}
		})
	}
}
