package schemas

func stringArray(description string) map[string]interface{} {
	return map[string]interface{}{
		"type":        "array",
		"description": description,
		"items":       map[string]interface{}{"type": "string"},
	}
}

func routesSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"description": "Map of service name to its route (the shipyard.route label value, e.g. \"/\" or \"/api\"). " +
			"Only routes to set: the plan's routes come from the compose file and already apply, and sending one back " +
			"duplicates it. To pick the main route, pass one of the plan's route_candidates with \"/\"",
		"additionalProperties": map[string]interface{}{"type": "string"},
	}
}

func envSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "array",
		"description": "Variables to set. Ask the user for each value; values are sent to Shipyard and never " +
			"returned in the result. kind=runtime (default) sets a secret, kind=build_arg a build arg",
		"items": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"name":  map[string]interface{}{"type": "string", "description": "Variable name"},
				"value": map[string]interface{}{"type": "string", "description": "Variable value"},
				"kind": map[string]interface{}{
					"type":        "string",
					"enum":        []string{"runtime", "build_arg"},
					"description": "runtime (default) or build_arg, as the plan's env list says",
				},
				"hidden": map[string]interface{}{"type": "boolean", "description": "Mask the value in API responses"},
				"services": stringArray("Services the variable reaches: the plan's services for it. Without them a value reaches " +
					"every service, so they are required"),
			},
			"required": []string{"name", "value", "services"},
		},
	}
}

// PlanApplicationSchema is the input for plan_application.
func PlanApplicationSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"repo":   map[string]interface{}{"type": "string", "description": "GitHub repo as owner/name"},
			"branch": map[string]interface{}{"type": "string", "description": "Branch to build, already pushed"},
		},
		"required": []string{"repo", "branch"},
	}
}

// CreateApplicationSchema is the input for create_application.
func CreateApplicationSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"repo":     map[string]interface{}{"type": "string", "description": "GitHub repo as owner/name"},
			"branch":   map[string]interface{}{"type": "string", "description": "Branch to build, already pushed"},
			"app_name": map[string]interface{}{"type": "string", "description": "Application name, unique in the org: pass the plan's app_name unless the user chose another (defaults to <repo>-<branch>)"},
			"services": stringArray("Compose services to enable, at least one: the plan's services with enabled_by_default, " +
				"plus any the user turns on"),
			"visitors": stringArray("GitHub logins who may open the environment. Required with an org API token " +
				"(next_step visitors_required); with a user token the caller is added"),
			"selected_profiles": stringArray("Compose profiles to turn on: each enabled service whose enabled_by_default " +
				"is false needs one of its profiles here"),
			"routes": routesSchema(),
			"env":    envSchema(),
			"head_sha": map[string]interface{}{
				"type":        "string",
				"description": "head_sha from plan_application, so Shipyard builds the commit the user confirmed and refuses a plan that went stale",
			},
			"compose_filename": map[string]interface{}{
				"type":        "string",
				"description": "Compose file path, when the plan's compose_file is not the default",
			},
		},
		"required": []string{"repo", "branch", "head_sha", "services"},
	}
}

// UpdateApplicationSchema is the input for update_application.
func UpdateApplicationSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"application_id": map[string]interface{}{"type": "string", "description": "Application UUID"},
			"projects": map[string]interface{}{
				"type":        "array",
				"description": "Repos to change or add. Only the fields given change; repos left out are kept",
				"items": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"repo":             map[string]interface{}{"type": "string", "description": "Repo name, or owner/name to add a repo"},
						"branch":           map[string]interface{}{"type": "string", "description": "Branch"},
						"services":         stringArray("Compose services to enable"),
						"routes":           routesSchema(),
						"compose_filename": map[string]interface{}{"type": "string", "description": "Compose file path"},
						"remove_routes":    stringArray("Services whose route to remove"),
						"selected_profiles": stringArray("Compose profiles to turn on: each enabled service whose " +
							"enabled_by_default is false needs one of its profiles here"),
					},
					"required": []string{"repo"},
				},
			},
			"remove_repos": stringArray("Bare repo names to unlink, as the app links them (not owner/name)"),
		},
		"required": []string{"application_id"},
	}
}
