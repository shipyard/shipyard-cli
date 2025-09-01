package schemas

// ListEnvironmentsSchema defines the input schema for listing environments
func ListEnvironmentsSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"branch": map[string]interface{}{
				"type":        "string",
				"description": "Filter by branch name",
			},
			"repo_name": map[string]interface{}{
				"type":        "string",
				"description": "Filter by repository name",
			},
			"deleted": map[string]interface{}{
				"type":        "boolean",
				"description": "Include deleted environments",
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
	}
}

// EnvironmentIDSchema defines the input schema for environment ID operations
func EnvironmentIDSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"environment_id": map[string]interface{}{
				"type":        "string",
				"description": "Environment ID",
			},
		},
		"required": []string{"environment_id"},
	}
}

// LogsSchema defines the input schema for logs operations
func LogsSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"environment_id": map[string]interface{}{
				"type":        "string",
				"description": "Environment ID",
			},
			"service_name": map[string]interface{}{
				"type":        "string",
				"description": "Service name",
			},
			"tail": map[string]interface{}{
				"type":        "integer",
				"description": "Number of lines to show from the end",
				"default":     100,
			},
			"page": map[string]interface{}{
				"type":        "integer",
				"description": "Page number for pagination (1-based)",
				"default":     1,
			},
			"page_size": map[string]interface{}{
				"type":        "integer",
				"description": "Number of log lines per page",
				"default":     20,
			},
		},
		"required": []string{"environment_id", "service_name"},
	}
}

// EmptySchema defines an empty input schema for operations that don't require parameters
func EmptySchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
	}
}

// OrgNameSchema defines the input schema for organization name operations
func OrgNameSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"org_name": map[string]interface{}{
				"type":        "string",
				"description": "Organization name",
			},
		},
		"required": []string{"org_name"},
	}
}

// ServiceExecSchema defines the input schema for executing commands in services
func ServiceExecSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"environment_id": map[string]interface{}{
				"type":        "string",
				"description": "Environment ID",
			},
			"service_name": map[string]interface{}{
				"type":        "string",
				"description": "Service name",
			},
			"command": map[string]interface{}{
				"type":        "array",
				"description": "Command and arguments to execute",
				"items":       map[string]interface{}{"type": "string"},
			},
		},
		"required": []string{"environment_id", "service_name", "command"},
	}
}

// ServicePortForwardSchema defines the input schema for port forwarding services
func ServicePortForwardSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"environment_id": map[string]interface{}{
				"type":        "string",
				"description": "Environment ID",
			},
			"service_name": map[string]interface{}{
				"type":        "string",
				"description": "Service name",
			},
			"ports": map[string]interface{}{
				"type":        "array",
				"description": "Port mappings in format 'local:remote' (e.g., '8080:80')",
				"items":       map[string]interface{}{"type": "string"},
			},
		},
		"required": []string{"environment_id", "service_name", "ports"},
	}
}

// SnapshotsListSchema defines the input schema for listing volume snapshots
func SnapshotsListSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"environment_id": map[string]interface{}{
				"type":        "string",
				"description": "Environment ID",
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

// VolumeResetSchema defines the input schema for resetting volumes
func VolumeResetSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"environment_id": map[string]interface{}{
				"type":        "string",
				"description": "Environment ID",
			},
			"volume_name": map[string]interface{}{
				"type":        "string",
				"description": "Volume name",
			},
		},
		"required": []string{"environment_id", "volume_name"},
	}
}

// SnapshotCreateSchema defines the input schema for creating snapshots
func SnapshotCreateSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"environment_id": map[string]interface{}{
				"type":        "string",
				"description": "Environment ID",
			},
			"note": map[string]interface{}{
				"type":        "string",
				"description": "An optional description of the snapshot",
			},
		},
		"required": []string{"environment_id"},
	}
}

// SnapshotLoadSchema defines the input schema for loading snapshots
func SnapshotLoadSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"environment_id": map[string]interface{}{
				"type":        "string",
				"description": "Environment ID",
			},
			"sequence_number": map[string]interface{}{
				"type":        "integer",
				"description": "Sequence number of the snapshot to load",
			},
			"source_application_id": map[string]interface{}{
				"type":        "string",
				"description": "Source application ID (optional)",
			},
		},
		"required": []string{"environment_id", "sequence_number"},
	}
}
