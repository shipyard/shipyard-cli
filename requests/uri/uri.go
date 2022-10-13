package uri

import (
	"fmt"
	"net/url"
)

const baseURL = "https://shipyard.build/api/v1"

func CreateResourceURI(action string, resource string, id string, params map[string]string) string {
	var query string
	if len(params) > 0 {
		query = buildQueryString(params)
	}
	if id == "" {
		return fmt.Sprintf("%s/%s%s", baseURL, resource, query)
	}
	if action == "" {
		return fmt.Sprintf("%s/%s/%s%s", baseURL, resource, id, query)
	}
	return fmt.Sprintf("%s/%s/%s/%s%s", baseURL, resource, id, action, query)
}

func buildQueryString(params map[string]string) string {
	var query string
	first := true
	for k, v := range params {
		if first {
			first = false
			query = query + "?"
		} else {
			query = query + "&"
		}
		val := url.QueryEscape(v)
		query = query + fmt.Sprintf("%s=%s", k, val)
	}
	return query
}
