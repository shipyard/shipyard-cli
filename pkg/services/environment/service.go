package environment

// TODO: Add interface for testability and mocking
// TODO: Consolidate Restart/Stop/Cancel/Rebuild/Revive into single executeAction method
// TODO: Move URI construction to client/repository layer
// TODO: Add context.Context to all methods for timeout/cancellation
// TODO: Add proper input validation beyond empty string checks
// TODO: Replace string-based error parsing with HTTP status code handling
// TODO: Add caching for GetByID operations
// TODO: Implement retry logic and circuit breaker patterns

import (
	"fmt"
	"net/http"

	"github.com/shipyard/shipyard-cli/pkg/client"
	"github.com/shipyard/shipyard-cli/pkg/requests/uri"
	"github.com/shipyard/shipyard-cli/pkg/types"
)

// EnvironmentManager handles environment business operations
type EnvironmentManager struct {
	client client.Client
}

// NewEnvironmentManager creates a new environment manager
func NewEnvironmentManager(client client.Client) *EnvironmentManager {
	return &EnvironmentManager{client: client}
}

// ListRequest contains parameters for listing environments
type ListRequest struct {
	Branch            string
	RepoName          string
	Deleted           bool
	Page              int
	PageSize          int
	Name              string
	OrgName           string
	PullRequestNumber string
}

// ListResponse contains the result of listing environments
type ListResponse struct {
	Environments []types.Environment
	HasNext      bool
	NextPage     int
	Links        types.Links
}

// List retrieves environments based on the provided filters
func (s *EnvironmentManager) List(req ListRequest) (*ListResponse, error) {
	// Build query parameters
	params := make(map[string]string)

	if req.Name != "" {
		params["name"] = req.Name
	}
	if req.OrgName != "" {
		params["org_name"] = req.OrgName
	}
	if req.RepoName != "" {
		params["repo_name"] = req.RepoName
	}
	if req.Branch != "" {
		params["branch"] = req.Branch
	}
	if req.PullRequestNumber != "" {
		params["pull_request_number"] = req.PullRequestNumber
	}
	if req.Deleted {
		params["deleted"] = "true"
	}
	if req.Page != 0 {
		params["page"] = fmt.Sprintf("%d", req.Page)
	}
	if req.PageSize != 0 {
		params["page_size"] = fmt.Sprintf("%d", req.PageSize)
	}
	if s.client.OrgLookupFn != nil {
		if org := s.client.OrgLookupFn(); org != "" {
			params["org"] = org
		}
	}

	// Make API call
	apiURI := uri.CreateResourceURI("", "environment", "", "", params)
	body, err := s.client.Requester.Do(http.MethodGet, apiURI, "application/json", nil)
	if err != nil {
		context := map[string]interface{}{
			"operation": "list_environments",
			"uri":       apiURI,
		}
		return nil, ParseAPIError(err, context)
	}

	// Parse response
	r, err := types.UnmarshalManyEnvs(body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse environments response (body: %s): %w", string(body), err)
	}

	// Build service response
	response := &ListResponse{
		Environments: r.Data,
		Links:        r.Links,
		HasNext:      r.Links.Next != "",
	}

	if response.HasNext {
		nextPage := req.Page + 1
		if req.Page == 0 {
			nextPage = 2 // Default page is 1, so next would be 2
		}
		response.NextPage = nextPage
	}

	return response, nil
}

// GetByID retrieves a single environment by its ID
func (s *EnvironmentManager) GetByID(id string) (*types.Environment, error) {
	if id == "" {
		return nil, fmt.Errorf("environment ID is required")
	}

	resp, err := s.client.EnvByID(id)
	if err != nil {
		context := map[string]interface{}{
			"operation":      "get_environment",
			"environment_id": id,
		}
		return nil, ParseAPIError(err, context)
	}

	return &resp.Data, nil
}

// Restart restarts a stopped environment
func (s *EnvironmentManager) Restart(id string) error {
	if id == "" {
		return fmt.Errorf("environment ID is required")
	}

	params := make(map[string]string)
	if s.client.OrgLookupFn != nil {
		if org := s.client.OrgLookupFn(); org != "" {
			params["org"] = org
		}
	}

	restartURI := uri.CreateResourceURI("restart", "environment", id, "", params)
	_, err := s.client.Requester.Do(http.MethodPost, restartURI, "application/json", nil)
	if err != nil {
		context := map[string]interface{}{
			"operation":      "restart_environment",
			"environment_id": id,
		}
		return ParseAPIError(err, context)
	}

	return nil
}

// Stop stops a running environment
func (s *EnvironmentManager) Stop(id string) error {
	if id == "" {
		return fmt.Errorf("environment ID is required")
	}

	params := make(map[string]string)
	if s.client.OrgLookupFn != nil {
		if org := s.client.OrgLookupFn(); org != "" {
			params["org"] = org
		}
	}

	stopURI := uri.CreateResourceURI("stop", "environment", id, "", params)
	_, err := s.client.Requester.Do(http.MethodPost, stopURI, "application/json", nil)
	if err != nil {
		context := map[string]interface{}{
			"operation":      "stop_environment",
			"environment_id": id,
		}
		return ParseAPIError(err, context)
	}

	return nil
}

// Cancel cancels an environment's latest build
func (s *EnvironmentManager) Cancel(id string) error {
	if id == "" {
		return fmt.Errorf("environment ID is required")
	}

	params := make(map[string]string)
	if s.client.OrgLookupFn != nil {
		if org := s.client.OrgLookupFn(); org != "" {
			params["org"] = org
		}
	}

	cancelURI := uri.CreateResourceURI("cancel", "environment", id, "", params)
	_, err := s.client.Requester.Do(http.MethodPost, cancelURI, "application/json", nil)
	if err != nil {
		context := map[string]interface{}{
			"operation":      "cancel_environment",
			"environment_id": id,
		}
		return ParseAPIError(err, context)
	}

	return nil
}

// Rebuild rebuilds an environment
func (s *EnvironmentManager) Rebuild(id string) error {
	if id == "" {
		return fmt.Errorf("environment ID is required")
	}

	params := make(map[string]string)
	if s.client.OrgLookupFn != nil {
		if org := s.client.OrgLookupFn(); org != "" {
			params["org"] = org
		}
	}

	rebuildURI := uri.CreateResourceURI("rebuild", "environment", id, "", params)
	_, err := s.client.Requester.Do(http.MethodPost, rebuildURI, "application/json", nil)
	if err != nil {
		context := map[string]interface{}{
			"operation":      "rebuild_environment",
			"environment_id": id,
		}
		return ParseAPIError(err, context)
	}

	return nil
}

// Revive revives a deleted environment
func (s *EnvironmentManager) Revive(id string) error {
	if id == "" {
		return fmt.Errorf("environment ID is required")
	}

	params := make(map[string]string)
	if s.client.OrgLookupFn != nil {
		if org := s.client.OrgLookupFn(); org != "" {
			params["org"] = org
		}
	}

	reviveURI := uri.CreateResourceURI("revive", "environment", id, "", params)
	_, err := s.client.Requester.Do(http.MethodPost, reviveURI, "application/json", nil)
	if err != nil {
		context := map[string]interface{}{
			"operation":      "revive_environment",
			"environment_id": id,
		}
		return ParseAPIError(err, context)
	}

	return nil
}
