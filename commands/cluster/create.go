package cluster

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/shipyard/shipyard-cli/pkg/client"
	"github.com/shipyard/shipyard-cli/pkg/requests/uri"
)

// Spinner for showing loading animation
type Spinner struct {
	chars []string
	pos   int
	done  chan bool
}

func NewSpinner() *Spinner {
	return &Spinner{
		chars: []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
		pos:   0,
		done:  make(chan bool),
	}
}

func (s *Spinner) Start(message string) {
	go func() {
		for {
			select {
			case <-s.done:
				return
			default:
				fmt.Printf("\r%s %s", s.chars[s.pos], message)
				s.pos = (s.pos + 1) % len(s.chars)
				time.Sleep(100 * time.Millisecond)
			}
		}
	}()
}

func (s *Spinner) Stop() {
	s.done <- true
	fmt.Print("\r\033[K") // Clear the line
}

// runCommandWithSpinner executes a command with optional spinner and verbose output
func runCommandWithSpinner(cmd *exec.Cmd, message string) error {
	isVerbose := viper.GetBool("verbose")

	if isVerbose {
		// In verbose mode, show all output
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	// In normal mode, show spinner
	spinner := NewSpinner()
	spinner.Start(message)

	// Capture output for error reporting
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	spinner.Stop()

	if err != nil {
		// Show error output even in non-verbose mode
		if stderr.Len() > 0 {
			fmt.Printf("Error output: %s\n", stderr.String())
		}
	}

	return err
}

type ClusterPreflightResponse struct {
	TailscaleOperatorFQDN   string `json:"tailscale_operator_fqdn"`
	TailscaleOAuthAppId     string `json:"tailscale_oauth_app_id"`
	TailscaleOAuthAppSecret string `json:"tailscale_oauth_app_secret"`
	TailscaleOperatorName   string `json:"tailscale_operator_name"`
	ClusterName             string `json:"sanitized_name"`
}

func NewCreateCmd(c *client.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:          "create",
		Short:        "Create a new local shipyard cluster",
		Long:         `Create a new local shipyard cluster.`,
		SilenceUsage: true,
		RunE:         runCreate(c),
	}

	cmd.Flags().String("api-port", "6443", "API server port")
	cmd.Flags().String("http-port", "80", "HTTP port")
	cmd.Flags().String("https-port", "443", "HTTPS port")

	return cmd
}

func runCreate(c *client.Client) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		green := color.New(color.FgHiGreen)
		blue := color.New(color.FgHiBlue)

		blue.Println("🚀 Creating Shipyard cluster...")

		// Step 1: Check and install dependencies
		blue.Println("📋 Checking system dependencies...")
		if err := ensureDependencies(); err != nil {
			return fmt.Errorf("failed to ensure dependencies: %w", err)
		}
		green.Println("✓ All dependencies are installed and ready")

		// Step 2: Get cluster initialization data from API
		blue.Println("🌐 Fetching org details from Shipyard...")
		clusterConfig, err := getClusterPreflightConfig(c, false)
		if err != nil {
			return fmt.Errorf("failed to get cluster configuration: %w", err)
		}
		green.Println("✓ Org details received")

		// Step 3: Create registry configuration
		blue.Println("📝 Creating registry configuration...")
		if err := createRegistryConfig(); err != nil {
			return fmt.Errorf("failed to create registry config: %w", err)
		}
		green.Println("✓ Registry configuration created")

		if err := createVolumesDirectory(); err != nil {
			return fmt.Errorf("failed to create volumes directory: %w", err)
		}

		// Step 5: Create k3d cluster
		clusterName := "org-" + clusterConfig.ClusterName
		apiPort, _ := cmd.Flags().GetString("api-port")
		httpPort, _ := cmd.Flags().GetString("http-port")
		httpsPort, _ := cmd.Flags().GetString("https-port")

		// Check if cluster already exists
		if clusterExists(clusterName) {
			blue.Printf("⚠️  Cluster '%s' already exists.\n", clusterName)
			if !confirmClusterDeletion(clusterName) {
				blue.Println("❌ Cluster creation cancelled.")
				return nil
			}

			blue.Printf("🗑️  Deleting existing cluster '%s'...\n", clusterName)
			if err := deleteCluster(clusterName); err != nil {
				return fmt.Errorf("failed to delete existing cluster: %w", err)
			}
			green.Printf("✓ Cluster '%s' deleted successfully\n", clusterName)
		}

		blue.Printf("🔧 Creating cluster '%s'...\n", clusterName)
		if err := createK3dCluster(clusterName, apiPort, httpPort, httpsPort, clusterConfig); err != nil {
			return fmt.Errorf("failed to create cluster: %w", err)
		}
		green.Printf("✓ Cluster '%s' created successfully\n", clusterName)

		// Step 6: Get and save kubeconfig
		blue.Println("⚙️  Retrieving kubeconfig...")
		if err := saveKubeconfig(clusterName); err != nil {
			return fmt.Errorf("failed to save kubeconfig: %w", err)
		}
		green.Println("✓ Kubeconfig saved")

		// Step 7: Install Tailscale operator via Helm
		blue.Println("🔧 Creating connection to Shipyard...")
		kubeconfigContent, err := createTailscaleOperator(clusterConfig)
		if err != nil {
			return fmt.Errorf("failed to create connection to Shipyard: %w", err)
		}
		green.Println("✓ Connection to Shipyard created successfully")

		// Step 8: Provision cluster with Shipyard
		blue.Println("🚀 Provisioning cluster with Shipyard...")
		if err := provisionCluster(c, kubeconfigContent); err != nil {
			return fmt.Errorf("failed to provision cluster: %w", err)
		}
		green.Println("✓ Cluster provisioned successfully")

		return nil
	}
}

func ensureDependencies() error {
	dependencies := []struct {
		name     string
		checkCmd []string
		install  func() error
	}{
		{
			name:     "k3d",
			checkCmd: []string{"k3d", "--version"},
			install:  installK3d,
		},
		{
			name:     "Tailscale",
			checkCmd: []string{"tailscale", "version"},
			install:  installTailscale,
		},
		{
			name:     "Helm",
			checkCmd: []string{"helm", "version"},
			install:  installHelm,
		},
	}

	for _, dep := range dependencies {
		if err := checkCommand(dep.checkCmd[0]); err != nil {
			if err := dep.install(); err != nil {
				return fmt.Errorf("failed to install %s: %w", dep.name, err)
			}
		}
	}

	return nil
}

func checkCommand(cmd string) error {
	_, err := exec.LookPath(cmd)
	return err
}

func installK3d() error {
	switch runtime.GOOS {
	case "darwin":
		cmd := exec.Command("brew", "install", "k3d")
		return runCommandWithSpinner(cmd, "Installing k3d via Homebrew...")
	case "linux":
		// Install k3d using the official install script
		cmd := exec.Command("wget", "-q", "-O", "-", "https://raw.githubusercontent.com/k3d-io/k3d/main/install.sh")
		installCmd := exec.Command("bash")
		installCmd.Stdin, _ = cmd.StdoutPipe()
		return runCommandWithSpinner(installCmd, "Installing k3d via install script...")
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

func installTailscale() error {
	switch runtime.GOOS {
	case "darwin":
		cmd := exec.Command("brew", "install", "tailscale")
		return runCommandWithSpinner(cmd, "Installing Tailscale via Homebrew...")
	case "linux":
		// Install Tailscale using the official install script
		cmd := exec.Command("curl", "-fsSL", "https://tailscale.com/install.sh", "-o", "install-tailscale.sh")
		if err := cmd.Run(); err != nil {
			return err
		}
		installCmd := exec.Command("sudo", "sh", "install-tailscale.sh")
		return runCommandWithSpinner(installCmd, "Installing Tailscale via install script...")
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

func installHelm() error {
	switch runtime.GOOS {
	case "darwin":
		cmd := exec.Command("brew", "install", "helm")
		return runCommandWithSpinner(cmd, "Installing Helm via Homebrew...")
	case "linux":
		// Install Helm using the official install script
		cmd := exec.Command("curl", "-fsSL", "https://get.helm.sh/helm-v3.12.0-linux-amd64.tar.gz", "-o", "helm.tar.gz")
		if err := cmd.Run(); err != nil {
			return err
		}
		extractCmd := exec.Command("tar", "-xzf", "helm.tar.gz")
		if err := runCommandWithSpinner(extractCmd, "Extracting Helm..."); err != nil {
			return err
		}
		moveCmd := exec.Command("sudo", "mv", "linux-amd64/helm", "/usr/local/bin/helm")
		if err := runCommandWithSpinner(moveCmd, "Moving Helm to /usr/local/bin..."); err != nil {
			return err
		}
		// Clean up
		exec.Command("rm", "-rf", "linux-amd64", "helm.tar.gz").Run()
		return nil
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

func getClusterPreflightConfig(c *client.Client, sync bool) (*ClusterPreflightResponse, error) {
	// Build parameters map with organization
	params := make(map[string]string)
	if org := c.OrgLookupFn(); org != "" {
		params["org"] = org
	}
	if sync {
		params["sync"] = "true"
	} else {
		params["sync"] = "false"
	}

	// Use CreateResourceURI to build the URL
	url := uri.CreateResourceURI("", "cluster/preflight", "", "", params)

	// Use the existing HTTP client from the requests package
	response, err := c.Requester.Do(http.MethodGet, url, "application/json", nil)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}

	var clusterConfig ClusterPreflightResponse
	if err := json.Unmarshal(response, &clusterConfig); err != nil {
		return nil, fmt.Errorf("failed to parse API response: %w", err)
	}

	if clusterConfig.TailscaleOperatorFQDN == "" || clusterConfig.TailscaleOAuthAppId == "" || clusterConfig.TailscaleOAuthAppSecret == "" {
		return nil, fmt.Errorf("invalid cluster configuration received from API")
	}

	return &clusterConfig, nil
}

func createRegistryConfig() error {
	// Create hack directory if it doesn't exist
	hackDir := "hack"
	if err := os.MkdirAll(hackDir, 0755); err != nil {
		return err
	}

	// Create registries.yaml
	registryConfig := `auths: null
configs: null
mirrors:
  10.43.0.16:
    endpoint:
      - http://10.43.0.16:5000
  10.43.0.16:5000:
    endpoint:
      - http://10.43.0.16:5000
`

	registryPath := filepath.Join(hackDir, "registries.yaml")
	return os.WriteFile(registryPath, []byte(registryConfig), 0644)
}

func createVolumesDirectory() error {
	volumesDir := "volumes/nfsdata"
	return os.MkdirAll(volumesDir, 0755)
}

func createK3dCluster(name, apiPort, httpPort, httpsPort string, config *ClusterPreflightResponse) error {
	// Set environment variables for the k3d command
	env := os.Environ()
	env = append(env, fmt.Sprintf("TAILSCALE_OPERATOR_FQDN=%s", config.TailscaleOperatorFQDN))

	// Get current working directory
	pwd, err := os.Getwd()
	if err != nil {
		return err
	}

	// Build k3d command
	args := []string{
		"cluster", "create", name,
		"--api-port", apiPort,
		"--port", fmt.Sprintf("%s:80@loadbalancer", httpPort),
		"--port", fmt.Sprintf("%s:443@loadbalancer", httpsPort),
		"--k3s-arg", "--disable=traefik@server:*",
		"--k3s-arg", "--service-cidr=10.43.0.0/16@server:*",
		"--k3s-arg", fmt.Sprintf("--tls-san=%s@server:*", config.TailscaleOperatorFQDN),
		"--registry-config", "hack/registries.yaml",
		"--volume", fmt.Sprintf("%s/volumes/nfsdata:/exports/nfs@all", pwd),
		"--runtime-label", "shipyard.managed=true@server:*",
		"--wait",
	}

	cmd := exec.Command("k3d", args...)
	cmd.Env = env
	return runCommandWithSpinner(cmd, "Creating shipyard cluster...")
}

func saveKubeconfig(clusterName string) error {
	// Get kubeconfig from k3d
	cmd := exec.Command("k3d", "kubeconfig", "get", clusterName)
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get kubeconfig: %w", err)
	}

	// Save to sy kubeconfig location
	kubeconfigPath := filepath.Join(os.Getenv("HOME"), ".kube", "sy-k3d-config")

	// Create .kube directory if it doesn't exist
	kubeDir := filepath.Dir(kubeconfigPath)
	if err := os.MkdirAll(kubeDir, 0755); err != nil {
		return err
	}

	// Write kubeconfig
	return os.WriteFile(kubeconfigPath, output, 0600)
}

func createTailscaleOperator(config *ClusterPreflightResponse) (string, error) {

	// Build helm command
	args := []string{
		"upgrade",
		"--install", "tailscale-operator",
		"tailscale/tailscale-operator",
		"--namespace", "tailscale", "--create-namespace",
		"--set-string", fmt.Sprintf("operatorConfig.hostname=%s", config.TailscaleOperatorName),
		"--set-string", "apiServerProxyConfig.mode=noauth",
		"--set", fmt.Sprintf("oauth.clientId=%s", config.TailscaleOAuthAppId),
		"--set", fmt.Sprintf("oauth.clientSecret=%s", config.TailscaleOAuthAppSecret),
		"--wait",
	}

	cmd := exec.Command("helm", args...)
	if err := runCommandWithSpinner(cmd, "Dailing home..."); err != nil {
		return "", err
	}

	// Create Tailscale service account and generate kubeconfig
	kubeconfigContent, err := createTailscaleServiceAccount(config)
	if err != nil {
		return "", fmt.Errorf("failed to create Tailscale service account: %w", err)
	}

	return kubeconfigContent, nil
}

func createTailscaleServiceAccount(config *ClusterPreflightResponse) (string, error) {
	// Set environment variables
	tailscaleNamespace := "tailscale"
	tailscaleServiceAccount := "tailscale-access"
	k3dClusterName := "org-" + config.ClusterName
	tailscaleOperatorFQDN := config.TailscaleOperatorFQDN

	// Create service account
	cmd := exec.Command("kubectl", "-n", tailscaleNamespace, "create", "serviceaccount", tailscaleServiceAccount)
	cmd.Stderr = os.Stderr
	_ = cmd.Run() // Ignore error if service account already exists

	// Create service account token secret
	tokenSecretYAML := fmt.Sprintf(`apiVersion: v1
kind: Secret
type: kubernetes.io/service-account-token
metadata:
  name: %s-token
  namespace: %s
  annotations:
    kubernetes.io/service-account.name: %s
`, tailscaleServiceAccount, tailscaleNamespace, tailscaleServiceAccount)

	cmd = exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(tokenSecretYAML)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to create service account token: %w", err)
	}

	// Create cluster role binding
	cmd = exec.Command("kubectl", "create", "clusterrolebinding", "tailscale-access-binding",
		"--clusterrole=cluster-admin",
		fmt.Sprintf("--serviceaccount=%s:%s", tailscaleNamespace, tailscaleServiceAccount))
	cmd.Stderr = os.Stderr
	_ = cmd.Run() // Ignore error if binding already exists

	return createKubeconfigFromServiceAccount(tailscaleNamespace, tailscaleServiceAccount, k3dClusterName, tailscaleOperatorFQDN)
}

// createKubeconfigFromServiceAccount creates a kubeconfig from an existing service account
func createKubeconfigFromServiceAccount(namespace, serviceAccount, clusterName, operatorFQDN string) (string, error) {
	// Get the service account token
	cmd := exec.Command("kubectl", "-n", namespace, "get", "secret",
		fmt.Sprintf("%s-token", serviceAccount), "-o", "jsonpath={.data.token}")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get service account token: %w", err)
	}

	// Decode the token
	tokenBytes, err := base64.StdEncoding.DecodeString(string(output))
	if err != nil {
		return "", fmt.Errorf("failed to decode service account token: %w", err)
	}
	serviceAccountToken := string(tokenBytes)

	// Extract the TLS certificate from the operator secret
	certCmd := exec.Command("kubectl", "-n", namespace, "get", "secret", "operator",
		"-o", fmt.Sprintf("jsonpath={.data.%s\\.crt}", strings.ReplaceAll(operatorFQDN, ".", "\\.")))
	certOutput, err := certCmd.Output()
	if err != nil {
		// Fallback: try to get certificate from k3s-serving secret in kube-system namespace
		fmt.Printf("Failed to get TLS certificate from operator secret, trying k3s-serving secret...\n")
		certCmd = exec.Command("kubectl", "-n", "kube-system", "get", "secret", "k3s-serving",
			"-o", "jsonpath={.data.tls\\.crt}")
		certOutput, err = certCmd.Output()
		if err != nil {
			return "", fmt.Errorf("failed to get TLS certificate from secrets: %w", err)
		}
	}

	// Decode the certificate
	certBytes, err := base64.StdEncoding.DecodeString(string(certOutput))
	if err != nil {
		return "", fmt.Errorf("failed to decode TLS certificate: %w", err)
	}

	// Encode certificate for kubeconfig (base64 again for the kubeconfig format)
	certBase64 := base64.StdEncoding.EncodeToString(certBytes)

	// Create kubeconfig with proper certificate
	kubeconfigYAML := fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- name: %s
  cluster:
    server: https://%s
    certificate-authority-data: %s
users:
- name: %s
  user:
    token: %s
contexts:
- name: %s
  context:
    cluster: %s
    user: %s
current-context: %s
`, clusterName, operatorFQDN, certBase64, serviceAccount, serviceAccountToken, clusterName, clusterName, serviceAccount, clusterName)

	return kubeconfigYAML, nil
}

func clusterExists(clusterName string) bool {
	cmd := exec.Command("k3d", "cluster", "list")
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	// Check if the cluster name appears in the list
	return strings.Contains(string(output), clusterName)
}

func confirmClusterDeletion(clusterName string) bool {
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("Do you want to delete the existing cluster '%s' and create a new one? (y/N): ", clusterName)

	response, err := reader.ReadString('\n')
	if err != nil {
		return false
	}

	response = strings.TrimSpace(strings.ToLower(response))
	return response == "y" || response == "yes"
}

func deleteCluster(clusterName string) error {
	cmd := exec.Command("k3d", "cluster", "delete", clusterName)
	return runCommandWithSpinner(cmd, "Deleting existing cluster...")
}

func provisionCluster(c *client.Client, kubeconfigContent string) error {

	// Encode Tailscale kubeconfig to base64
	base64TailscaleKubeconfig := base64.StdEncoding.EncodeToString([]byte(kubeconfigContent))

	// Build parameters map with organization
	params := make(map[string]string)
	if org := c.OrgLookupFn(); org != "" {
		params["org"] = org
	}

	// Create the request body
	requestBody := map[string]string{
		"kubeconfig": base64TailscaleKubeconfig,
	}

	// Use CreateResourceURI to build the URL
	url := uri.CreateResourceURI("", "cluster/provision", "", "", params)

	// Make the POST request
	_, err := c.Requester.Do(http.MethodPost, url, "application/json", requestBody)
	if err != nil {
		return fmt.Errorf("failed to provision cluster: %w", err)
	}

	return nil
}
