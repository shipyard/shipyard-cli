package org

import (
	"fmt"
	"net/http"

	"github.com/shipyard/shipyard-cli/pkg/client"
	"github.com/shipyard/shipyard-cli/pkg/requests/uri"
	"github.com/shipyard/shipyard-cli/pkg/types"
	"github.com/spf13/viper"
)

// OrganizationManager handles organization business operations
type OrganizationManager struct {
	client client.Client
}

// NewOrganizationManager creates a new organization manager
func NewOrganizationManager(client client.Client) *OrganizationManager {
	return &OrganizationManager{client: client}
}

// List retrieves all organizations for the user
func (s *OrganizationManager) List() ([]string, error) {
	body, err := s.client.Requester.Do(http.MethodGet, uri.CreateResourceURI("", "org", "", "", nil), "application/json", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get organizations: %w", err)
	}

	orgs, err := types.UnmarshalOrgs(body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse organizations response: %w", err)
	}

	names := make([]string, 0, len(orgs.Data))
	for _, item := range orgs.Data {
		names = append(names, item.Attributes.Name)
	}

	return names, nil
}

// GetCurrent returns the currently configured organization
func (s *OrganizationManager) GetCurrent() (string, error) {
	org := viper.GetString("org")
	if org == "" {
		return "", fmt.Errorf("no org is found in the config")
	}
	return org, nil
}

// SetCurrent sets the default organization in config
func (s *OrganizationManager) SetCurrent(name string) error {
	if name == "" {
		return fmt.Errorf("organization name cannot be empty")
	}

	viper.Set("org", name)
	if err := viper.MergeInConfig(); err != nil {
		return fmt.Errorf("failed to merge config: %w", err)
	}

	if err := viper.WriteConfig(); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}
