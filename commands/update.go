package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/shipyard/shipyard-cli/version"
)

const (
	githubAPIBaseURL = "https://api.github.com"
	repoOwner        = "shipyard"
	repoName         = "shipyard-cli"
)

type GitHubRelease struct {
	TagName    string `json:"tag_name"`
	Name       string `json:"name"`
	Body       string `json:"body"`
	Prerelease bool   `json:"prerelease"`
	Assets     []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

func NewUpdateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update shipyard CLI to the latest version",
		Long:  `Check for the latest release on GitHub and update the CLI binary if a newer version is available.`,
		RunE:  runUpdate,
	}

	cmd.Flags().BoolP("force", "f", false, "Force update even if already on latest version")
	cmd.Flags().BoolP("prerelease", "p", false, "Include prerelease versions")

	return cmd
}

func runUpdate(cmd *cobra.Command, args []string) error {
	force, _ := cmd.Flags().GetBool("force")
	includePrerelease, _ := cmd.Flags().GetBool("prerelease")

	green := color.New(color.FgHiGreen)
	yellow := color.New(color.FgHiYellow)
	blue := color.New(color.FgHiBlue)

	_, _ = blue.Println("Checking for updates...")

	// Get current version
	currentVersion := version.Version
	if currentVersion == "undefined" {
		return fmt.Errorf("unable to determine current version")
	}

	// Fetch latest release from GitHub
	latestRelease, err := getLatestRelease(includePrerelease)
	if err != nil {
		return fmt.Errorf("failed to fetch latest release: %w", err)
	}

	_, _ = blue.Printf("Current version: %s\n", currentVersion)
	_, _ = blue.Printf("Latest version: %s\n", latestRelease.TagName)

	// Check if update is needed
	if !force && !isNewerVersion(currentVersion, latestRelease.TagName) {
		_, _ = green.Println("✓ You're already running the latest version!")
		return nil
	}

	// Find the appropriate asset for the current platform
	assetURL, err := findAssetForPlatform(latestRelease.Assets)
	if err != nil {
		return fmt.Errorf("failed to find compatible release asset: %w", err)
	}

	_, _ = yellow.Printf("Downloading %s...\n", latestRelease.TagName)

	// Download the new binary
	tempFile, err := downloadBinary(assetURL)
	if err != nil {
		return fmt.Errorf("failed to download binary: %w", err)
	}
	defer func() { _ = os.Remove(tempFile) }()

	// Get the current executable path
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	// Make the downloaded binary executable
	if err := os.Chmod(tempFile, 0755); err != nil {
		return fmt.Errorf("failed to make binary executable: %w", err)
	}

	// Create backup of current binary
	backupPath := execPath + ".backup"
	if err := copyFile(execPath, backupPath); err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}

	// Replace the current binary
	if err := copyFile(tempFile, execPath); err != nil {
		// Restore backup on failure
		_ = copyFile(backupPath, execPath)
		return fmt.Errorf("failed to update binary: %w", err)
	}

	// Remove backup file
	_ = os.Remove(backupPath)

	_, _ = green.Printf("✓ Successfully updated to %s!\n", latestRelease.TagName)
	_, _ = blue.Println("Please restart your terminal or run 'shipyard --version' to verify the update.")

	return nil
}

func getLatestRelease(includePrerelease bool) (*GitHubRelease, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases/latest", githubAPIBaseURL, repoOwner, repoName)

	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	// Add User-Agent header to avoid rate limiting
	req.Header.Set("User-Agent", "shipyard-cli-updater")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}

	// If we don't want prereleases and the latest is a prerelease, try to get the latest stable
	if !includePrerelease && release.Prerelease {
		// Get all releases and find the latest non-prerelease
		url = fmt.Sprintf("%s/repos/%s/%s/releases", githubAPIBaseURL, repoOwner, repoName)
		req, err = http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "shipyard-cli-updater")

		resp, err = client.Do(req)
		if err != nil {
			return nil, err
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
		}

		var releases []GitHubRelease
		if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
			return nil, err
		}

		// Find the first non-prerelease release
		for _, r := range releases {
			if !r.Prerelease {
				release = r
				break
			}
		}
	}

	return &release, nil
}

func findAssetForPlatform(assets []struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}) (string, error) {
	osName := runtime.GOOS
	arch := runtime.GOARCH

	// Map Go arch to common release asset arch names
	archMap := map[string]string{
		"amd64": "x86_64",
		"arm64": "arm64",
		"386":   "i386",
	}

	releaseArch := archMap[arch]
	if releaseArch == "" {
		releaseArch = arch
	}

	// Look for asset matching our platform
	expectedName := fmt.Sprintf("shipyard-%s-%s", osName, releaseArch)

	for _, asset := range assets {
		if strings.Contains(asset.Name, expectedName) {
			return asset.BrowserDownloadURL, nil
		}
	}

	// Fallback: look for any asset with our OS
	for _, asset := range assets {
		if strings.Contains(asset.Name, osName) {
			return asset.BrowserDownloadURL, nil
		}
	}

	return "", fmt.Errorf("no compatible release asset found for %s/%s", osName, arch)
}

func downloadBinary(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	// Create temporary file
	tempFile, err := os.CreateTemp("", "shipyard-update-*")
	if err != nil {
		return "", err
	}
	defer func() { _ = tempFile.Close() }()

	// Download to temp file
	_, err = io.Copy(tempFile, resp.Body)
	if err != nil {
		_ = os.Remove(tempFile.Name())
		return "", err
	}

	return tempFile.Name(), nil
}

func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = sourceFile.Close() }()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = destFile.Close() }()

	_, err = io.Copy(destFile, sourceFile)
	return err
}

func isNewerVersion(current, latest string) bool {
	// Remove 'v' prefix if present
	current = strings.TrimPrefix(current, "v")
	latest = strings.TrimPrefix(latest, "v")

	// Simple version comparison - this could be enhanced with proper semver parsing
	// For now, we'll do a basic string comparison
	return latest > current
}
