package services

import (
	"fmt"
	"sort"

	"github.com/agnivade/levenshtein"
	"github.com/spf13/viper"

	"github.com/shipyard/shipyard-cli/types"
)

func GetByName(serviceName string) (*types.Service, error) {
	if serviceName == "" {
		return nil, fmt.Errorf("service name not provided")
	}
	envID := viper.GetString("env")
	if envID == "" {
		return nil, fmt.Errorf("environment ID not provided")
	}

	svcs, err := GetAllByEnvironment(envID)
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

func findService(coll []types.Service, unsanitizedName string) *types.Service {
	for i := range coll {
		if coll[i].Name == unsanitizedName {
			return &coll[i]
		}
	}
	return nil
}

func similarServiceName(coll []types.Service, unsanitizedName string) string {
	if len(coll) == 0 || unsanitizedName == "" {
		return ""
	}

	type entry struct {
		name     string
		distance int
	}

	entries := make([]entry, len(coll))
	for i := range coll {
		entries[i].name = coll[i].Name
		entries[i].distance = levenshtein.ComputeDistance(unsanitizedName, entries[i].name)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].distance < entries[j].distance
	})
	return entries[0].name
}
