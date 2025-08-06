package uri

import (
	"fmt"
	"net/url"
	"sort"

	"github.com/spf13/viper"
)

func CreateResourceURI(action, resource, id, subresource string, params map[string]string) string {
	baseURL := "https://shipyard.build/api/v1"
	if value := viper.GetString("api_url"); value != "" {
		baseURL = value
	}

	var uri string

	switch {
	case id == "":
		uri = fmt.Sprintf("%s/%s", baseURL, resource)
	case subresource != "":
		uri = fmt.Sprintf("%s/%s/%s/%s", baseURL, resource, id, subresource)
	case action == "":
		uri = fmt.Sprintf("%s/%s/%s", baseURL, resource, id)
	default:
		uri = fmt.Sprintf("%s/%s/%s/%s", baseURL, resource, id, action)
	}

	return uri + buildQueryString(params)
}

// buildQueryString builds a URL-encoded query string from a map of parameters.
// The keys of the map are sorted alphabetically for deterministic
// behavior needed for testing, since Go's maps do not define an order of entries.
func buildQueryString(params map[string]string) string {
	if len(params) == 0 {
		return ""
	}

	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var query string
	first := true
	for _, k := range keys {
		if first {
			first = false
			query += "?"
		} else {
			query += "&"
		}
		val := url.QueryEscape(params[k])
		query += fmt.Sprintf("%s=%s", k, val)
	}
	return query
}
