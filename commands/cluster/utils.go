package cluster

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// ClusterInfo represents the structure of k3d cluster list JSON output
type ClusterInfo struct {
	Name   string            `json:"name"`
	Labels map[string]string `json:"labels,omitempty"`
	Nodes  []NodeInfo        `json:"nodes,omitempty"`
}

// NodeInfo represents a node in the cluster
type NodeInfo struct {
	Name          string            `json:"name"`
	Role          string            `json:"role"`
	Labels        map[string]string `json:"labels,omitempty"`
	RuntimeLabels map[string]string `json:"runtimeLabels,omitempty"`
}

// GetShipyardClusters returns a list of cluster names that are managed by Shipyard
func GetShipyardClusters() ([]string, error) {
	cmd := exec.Command("k3d", "cluster", "list", "--no-headers")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list clusters: %w", err)
	}

	// Use a map to track unique cluster names
	clusterMap := make(map[string]bool)
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}

		// Split by whitespace and get the first column (cluster name)
		fields := strings.Fields(line)
		if len(fields) > 0 {
			clusterName := fields[0]
			// Check if cluster has the Shipyard managed label
			if IsShipyardManagedCluster(clusterName) {
				clusterMap[clusterName] = true
			}
		}
	}

	// Convert map keys to slice for unique cluster names
	var clusters []string
	for clusterName := range clusterMap {
		clusters = append(clusters, clusterName)
	}

	return clusters, nil
}

// IsShipyardManagedCluster checks if a cluster was created by Shipyard CLI
func IsShipyardManagedCluster(clusterName string) bool {
	// First, try to check Docker labels directly since k3d JSON output doesn't include all runtime labels
	cmd := exec.Command("docker", "ps", "-a", "--filter", fmt.Sprintf("label=k3d.cluster=%s", clusterName), "--filter", "label=shipyard.managed=true", "--format", "{{.Names}}")
	output, err := cmd.Output()
	if err == nil && len(strings.TrimSpace(string(output))) > 0 {
		return true
	}

	// Fallback: Get cluster info in JSON format to check labels
	cmd = exec.Command("k3d", "cluster", "list", "-o", "json")
	output, err = cmd.Output()
	if err != nil {
		// Fallback to name-based check if JSON output fails
		return strings.HasPrefix(clusterName, "k3d-org-")
	}

	// Parse JSON output to check for labels
	var clusterList []ClusterInfo
	if err := json.Unmarshal(output, &clusterList); err != nil {
		// Fallback to name-based check if JSON parsing fails
		return strings.HasPrefix(clusterName, "k3d-org-")
	}

	// Find the cluster by name and check its labels
	for _, cluster := range clusterList {
		if cluster.Name == clusterName {
			// Check if the cluster has the Shipyard managed label in cluster labels
			if value, exists := cluster.Labels["shipyard.managed"]; exists && value == "true" {
				return true
			}

			// Check if any server node has the Shipyard managed runtime label
			for _, node := range cluster.Nodes {
				if node.Role == "server" {
					if value, exists := node.RuntimeLabels["shipyard.managed"]; exists && value == "true" {
						return true
					}
				}
			}

			// Also check the name prefix as a fallback
			return strings.HasPrefix(clusterName, "k3d-org-")
		}
	}

	// If cluster not found in JSON, fallback to name-based check
	return strings.HasPrefix(clusterName, "k3d-org-")
}

// DebugClusterLabels is a helper function to debug cluster labels
func DebugClusterLabels() error {
	cmd := exec.Command("k3d", "cluster", "list", "-o", "json")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get cluster list: %w", err)
	}

	var clusterList []ClusterInfo
	if err := json.Unmarshal(output, &clusterList); err != nil {
		return fmt.Errorf("failed to parse cluster list: %w", err)
	}

	fmt.Println("Cluster labels debug info:")
	for _, cluster := range clusterList {
		fmt.Printf("  Cluster: %s\n", cluster.Name)
		if len(cluster.Labels) > 0 {
			for key, value := range cluster.Labels {
				fmt.Printf("    Cluster Label: %s = %s\n", key, value)
			}
		} else {
			fmt.Printf("    No cluster labels\n")
		}

		for _, node := range cluster.Nodes {
			fmt.Printf("    Node: %s (Role: %s)\n", node.Name, node.Role)
			if len(node.Labels) > 0 {
				for key, value := range node.Labels {
					fmt.Printf("      Node Label: %s = %s\n", key, value)
				}
			}
			if len(node.RuntimeLabels) > 0 {
				for key, value := range node.RuntimeLabels {
					fmt.Printf("      Runtime Label: %s = %s\n", key, value)
				}
			}
			if len(node.Labels) == 0 && len(node.RuntimeLabels) == 0 {
				fmt.Printf("      No node labels\n")
			}
		}
	}

	return nil
}
