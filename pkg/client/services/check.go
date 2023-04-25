package services

import (
	"sort"

	"github.com/agnivade/levenshtein"
	"github.com/shipyard/shipyard-cli/pkg/types"
)

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
