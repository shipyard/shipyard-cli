package validation

import (
	"fmt"
	"regexp"
	"strings"
)

// TODO: validate UUID format using "github.com/google/uuid" package
// validateEnvironmentID validates that an environment ID has the expected format
func ValidateEnvironmentID(id string) error {
	if id == "" {
		return fmt.Errorf("environment_id is required. Example: 'my-environment-123'")
	}

	// Environment IDs should be non-empty strings with reasonable length
	if len(id) < 3 {
		return fmt.Errorf("environment_id too short (minimum 3 characters). Provided: '%s'", id)
	}

	if len(id) > 100 {
		return fmt.Errorf("environment_id too long (maximum 100 characters). Provided length: %d", len(id))
	}

	// Check for invalid characters (allow alphanumeric, hyphens, underscores)
	validID := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	if !validID.MatchString(id) {
		return fmt.Errorf("environment_id '%s' contains invalid characters. Only alphanumeric characters, hyphens (-), and underscores (_) are allowed", id)
	}

	return nil
}

// ValidateServiceName validates service name format
func ValidateServiceName(name string) error {
	if name == "" {
		return fmt.Errorf("service_name is required. Example: 'web', 'api', 'database'")
	}

	if len(name) < 1 {
		return fmt.Errorf("service_name cannot be empty")
	}

	if len(name) > 100 {
		return fmt.Errorf("service_name '%s' too long (maximum 100 characters). Provided length: %d", name, len(name))
	}

	// Service names should follow similar naming conventions
	validName := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	if !validName.MatchString(name) {
		return fmt.Errorf("service_name '%s' contains invalid characters. Only alphanumeric characters, hyphens (-), and underscores (_) are allowed", name)
	}

	return nil
}

// ValidateRepoName validates repository name format
func ValidateRepoName(name string) error {
	if name == "" {
		return nil // Optional field
	}

	if len(name) > 200 {
		return fmt.Errorf("repo_name '%s' too long (maximum 200 characters). Provided length: %d", name, len(name))
	}

	// Repository names can contain slashes for org/repo format
	validRepo := regexp.MustCompile(`^[a-zA-Z0-9_.-]+(/[a-zA-Z0-9_.-]+)?$`)
	if !validRepo.MatchString(name) {
		return fmt.Errorf("repo_name '%s' has invalid format. Expected format: 'repo-name' or 'org/repo-name'. Examples: 'my-app', 'acme/web-service'", name)
	}

	return nil
}

// ValidateBranchName validates git branch name format
func ValidateBranchName(name string) error {
	if name == "" {
		return nil // Optional field
	}

	if len(name) > 250 {
		return fmt.Errorf("branch name '%s' too long (maximum 250 characters). Provided length: %d", name, len(name))
	}

	// Branch names have specific git restrictions
	if strings.HasPrefix(name, "/") || strings.HasSuffix(name, "/") {
		return fmt.Errorf("branch name '%s' cannot start or end with slash. Examples: 'main', 'feature/new-ui', 'bugfix-123'", name)
	}

	if strings.Contains(name, "..") {
		return fmt.Errorf("branch name '%s' cannot contain consecutive dots (..). This is a Git restriction", name)
	}

	// Check for other invalid characters
	invalidChars := regexp.MustCompile(`[\x00-\x1f\x7f ~^:?*\[]`)
	if invalidChars.MatchString(name) {
		return fmt.Errorf("branch name '%s' contains invalid characters. Avoid control characters, spaces, tildes (~), carets (^), colons (:), question marks (?), asterisks (*), and brackets ([])", name)
	}

	return nil
}

// ValidatePagination validates pagination parameters
func ValidatePagination(page, pageSize int) error {
	if page < 0 {
		return fmt.Errorf("page must be non-negative. Provided: %d. Use page >= 1 for pagination", page)
	}

	if page == 0 {
		// Allow 0 as it gets converted to 1 as default
		return nil
	}

	if page > 10000 {
		return fmt.Errorf("page %d too large (maximum 10000). For large datasets, consider filtering results first", page)
	}

	if pageSize < 0 {
		return fmt.Errorf("page_size must be non-negative. Provided: %d. Use page_size >= 1", pageSize)
	}

	if pageSize == 0 {
		// Allow 0 as it gets converted to default
		return nil
	}

	if pageSize > 1000 {
		return fmt.Errorf("page_size %d too large (maximum 1000). Large page sizes may cause timeouts", pageSize)
	}

	return nil
}

// ValidateLogTail validates tail parameter for logs
func ValidateLogTail(tail int) error {
	if tail < 0 {
		return fmt.Errorf("tail must be non-negative. Provided: %d. Use tail >= 0 to show recent log lines", tail)
	}

	if tail > 10000 {
		return fmt.Errorf("tail %d too large (maximum 10000 lines). Large tail values may cause timeouts. Consider using pagination instead", tail)
	}

	return nil
}
