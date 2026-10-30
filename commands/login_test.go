package commands

import (
	"testing"

	"github.com/spf13/viper"
)

func TestLoginURL(t *testing.T) {
	t.Cleanup(func() { viper.Set("api_url", nil) })

	viper.Set("api_url", nil)
	if got, want := loginURL("localhost:5000"), "https://shipyard.build/api/me/user-token/cli?callbackUrl=http://localhost:5000"; got != want {
		t.Errorf("default: got %q, want %q", got, want)
	}

	// A self-hosted install logs in against its own host, never shipyard.build.
	viper.Set("api_url", "https://shipyard.example.internal/api/v1")
	if got, want := loginURL("localhost:5000"), "https://shipyard.example.internal/api/me/user-token/cli?callbackUrl=http://localhost:5000"; got != want {
		t.Errorf("api_url set: got %q, want %q", got, want)
	}
}
