package uri

import "fmt"

const baseURL = "https://shipyard.build/api/v1"

func CreateResourceURI(action string, resource string, id string) string {
	if id == "" {
		return fmt.Sprintf("%s/%s", baseURL, resource)
	}
	if action == "" {
		return fmt.Sprintf("%s/%s/%s", baseURL, resource, id)
	}
	return fmt.Sprintf("%s/%s/%s/%s", baseURL, resource, id, action)
}
