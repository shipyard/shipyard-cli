package cluster

import (
	"encoding/base64"
	"fmt"
	"net/http"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/shipyard/shipyard-cli/pkg/client"
	"github.com/shipyard/shipyard-cli/pkg/requests/uri"
)

func NewSyncCmd(c *client.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:          "sync",
		Short:        "Sync local cluster with Shipyard",
		Long:         `Sync the local cluster's kubeconfig with the Shipyard.`,
		SilenceUsage: true,
		RunE:         runSync(c),
	}

	return cmd
}

func runSync(c *client.Client) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		green := color.New(color.FgHiGreen)
		blue := color.New(color.FgHiBlue)

		blue.Println("🔄 Syncing Shipyard cluster...")

		// Step 2: Get org details to get the operator FQDN
		blue.Println("🌐 Fetching current cluster details from Shipyard...")
		clusterConfig, err := getClusterPreflightConfig(c, true)
		if err != nil {
			return fmt.Errorf("failed to get cluster configuration: %w", err)
		}
		green.Println("✓ Org details received")

		// Step 3: Create kubeconfig from existing service account
		blue.Println("📋 Getting local cluster details...")
		kubeconfigContent, err := createKubeconfigFromServiceAccount("tailscale", "tailscale-access", "org-"+clusterConfig.ClusterName, clusterConfig.TailscaleOperatorFQDN)
		if err != nil {
			return fmt.Errorf("failed to get kubeconfig: %w", err)
		}
		green.Println("✓ Local cluster details retrieved successfully")

		// Step 4: Sync cluster with Shipyard
		blue.Println("🔄 Syncing cluster with Shipyard...")
		if err := syncCluster(c, kubeconfigContent); err != nil {
			return fmt.Errorf("failed to sync cluster: %w", err)
		}
		green.Println("✓ Cluster synced successfully")

		return nil
	}
}

// syncCluster sends the kubeconfig to the core API to sync the cluster
func syncCluster(c *client.Client, kubeconfigContent string) error {
	// Encode kubeconfig to base64
	base64Kubeconfig := base64.StdEncoding.EncodeToString([]byte(kubeconfigContent))

	// Build parameters map with organization
	params := make(map[string]string)
	if org := c.OrgLookupFn(); org != "" {
		params["org"] = org
	}

	// Create the request body
	requestBody := map[string]string{
		"kubeconfig": base64Kubeconfig,
	}

	// Use CreateResourceURI to build the URL
	url := uri.CreateResourceURI("", "cluster/sync", "", "", params)

	// Make the POST request
	_, err := c.Requester.Do(http.MethodPost, url, "application/json", requestBody)
	if err != nil {
		return fmt.Errorf("failed to sync cluster: %w", err)
	}

	return nil
}
