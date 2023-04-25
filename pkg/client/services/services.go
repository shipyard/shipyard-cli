package services

import (
	"fmt"

	"github.com/shipyard/shipyard-cli/pkg/client/env"
	"github.com/shipyard/shipyard-cli/pkg/types"
)

// GetByName tries to fetch a single service.
func GetByName(serviceName, envID, org string) (*types.Service, error) {
	if serviceName == "" {
		return nil, fmt.Errorf("service name not provided")
	}
	if envID == "" {
		return nil, fmt.Errorf("environment ID not provided")
	}

	svcs, err := GetAll(envID, org)
	if err != nil {
		return nil, err
	}
	s := findService(svcs, serviceName)
	if s == nil {
		return nil, fmt.Errorf("service %s is not found, but there is a service named %s",
			serviceName, similarServiceName(svcs, serviceName))
	}
	return s, nil
}

// GetAll tries to fetch an environment's services.
func GetAll(id, org string) ([]types.Service, error) {
	if id == "" {
		return nil, fmt.Errorf("environment ID is missing")
	}

	environment, err := env.GetByID(id, org)
	if err != nil {
		return nil, err
	}

	services := environment.Data.Attributes.Services
	if len(services) == 0 {
		return nil, fmt.Errorf("no services found for environment, check if it's running")
	}
	return services, nil
}
