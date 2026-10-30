package commands

import (
	"errors"
	"testing"

	"github.com/spf13/viper"

	"github.com/shipyard/shipyard-cli/version"
)

// With self_update off, upgrade refuses before it looks anything up: the
// release build and unreachable GitHub URL would fail any other way.
func TestUpgradeRefusesWhenSelfUpdateOff(t *testing.T) {
	old := version.Version
	version.Version = "1.9.0"
	t.Cleanup(func() {
		version.Version = old
		viper.Set("self_update", nil)
	})

	viper.Set("self_update", false)
	if err := runUpgrade(NewUpgradeCmd(), nil); !errors.Is(err, errSelfUpdateOff) {
		t.Errorf("self_update off: got %v, want errSelfUpdateOff", err)
	}

	viper.Set("self_update", nil)
	t.Setenv("SHIPYARD_SELF_UPDATE", "false")
	if err := runUpgrade(NewUpgradeCmd(), nil); !errors.Is(err, errSelfUpdateOff) {
		t.Errorf("SHIPYARD_SELF_UPDATE=false: got %v, want errSelfUpdateOff", err)
	}
}
