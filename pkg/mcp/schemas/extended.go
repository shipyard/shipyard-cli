package schemas

// BuildHistorySchema is the input for get_build_history.
func BuildHistorySchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"environment_id": map[string]interface{}{
				"type":        "string",
				"description": "Environment ID",
			},
			"successfully_built": map[string]interface{}{
				"type":        "boolean",
				"description": "When true, only return builds that deployed successfully",
			},
			"page": map[string]interface{}{
				"type":    "integer",
				"default": 1,
			},
			"page_size": map[string]interface{}{
				"type":    "integer",
				"default": 20,
			},
		},
		"required": []string{"environment_id"},
	}
}

// GetEnvVarsSchema is the input for get_env_vars.
func GetEnvVarsSchema() map[string]interface{} {
	return EnvironmentIDSchema()
}

// PutEnvVarsSchema is the input for put_env_vars.
func PutEnvVarsSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"environment_id": map[string]interface{}{
				"type":        "string",
				"description": "Environment ID",
			},
			"env_vars": map[string]interface{}{
				"type":        "array",
				"description": "Env vars to create or update (last duplicate name wins)",
				"items": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"name": map[string]interface{}{
							"type":        "string",
							"description": "Variable name",
						},
						"value": map[string]interface{}{
							"type":        "string",
							"description": "Variable value",
						},
						"hidden": map[string]interface{}{
							"type":        "boolean",
							"description": "Mask the value in API responses (default true)",
						},
						"services": map[string]interface{}{
							"type":        "array",
							"description": "Optional service names to scope the variable",
							"items":       map[string]interface{}{"type": "string"},
						},
					},
					"required": []string{"name", "value"},
				},
			},
		},
		"required": []string{"environment_id", "env_vars"},
	}
}

// DeleteEnvVarSchema is the input for delete_env_var.
func DeleteEnvVarSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"environment_id": map[string]interface{}{
				"type":        "string",
				"description": "Environment ID",
			},
			"name": map[string]interface{}{
				"type":        "string",
				"description": "Environment variable name to delete",
			},
		},
		"required": []string{"environment_id", "name"},
	}
}

// RestartServiceSchema is the input for restart_service.
func RestartServiceSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"environment_id": map[string]interface{}{
				"type":        "string",
				"description": "Environment ID",
			},
			"service_name": map[string]interface{}{
				"type":        "string",
				"description": "Service name to restart",
			},
		},
		"required": []string{"environment_id", "service_name"},
	}
}

// DeployDetachedSchema is the input for deploy_detached.
func DeployDetachedSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"application_build_id": map[string]interface{}{
				"type":        "string",
				"description": "Source application build UUID to clone",
			},
			"display_name": map[string]interface{}{
				"type":        "string",
				"description": "Display name for the new detached environment",
			},
			"project_branch_overrides": map[string]interface{}{
				"type":                 "object",
				"description":          "Per-repo branch overrides as {repo_name: branch}",
				"additionalProperties": map[string]interface{}{"type": "string"},
			},
			"build_on_commit": map[string]interface{}{
				"description": "Rebuild policy: 'always', 'inherit', 'never', or per-repo object {repo_name: setting}",
			},
		},
		"required": []string{"application_build_id"},
	}
}

// UpdateBranchesSchema is the input for update_branches.
func UpdateBranchesSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"environment_id": map[string]interface{}{
				"type":        "string",
				"description": "Environment ID",
			},
			"projects": map[string]interface{}{
				"type":        "array",
				"description": "Full list of repo/branch pairs for every repo in the environment (partial lists are rejected by the API)",
				"items": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"repo_name": map[string]interface{}{
							"type":        "string",
							"description": "Repository name",
						},
						"branch": map[string]interface{}{
							"type":        "string",
							"description": "Branch to deploy",
						},
					},
					"required": []string{"repo_name", "branch"},
				},
			},
		},
		"required": []string{"environment_id", "projects"},
	}
}
