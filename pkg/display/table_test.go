package display

import (
	"testing"

	"github.com/spf13/viper"
)

func TestEnvironmentDetailsURL(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	if got, want := environmentDetailsURL("abc"), "https://shipyard.build/application/abc/detail"; got != want {
		t.Errorf("default: got %q, want %q", got, want)
	}

	viper.Set("api_url", "https://shipyard.example.internal/api/v1")
	if got, want := environmentDetailsURL("abc"), "https://shipyard.example.internal/application/abc/detail"; got != want {
		t.Errorf("api_url set: got %q, want %q", got, want)
	}
}
