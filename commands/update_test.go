package commands

import (
	"errors"
	"testing"

	"github.com/spf13/viper"

	"github.com/shipyard/shipyard-cli/version"
)

// With self_update off, upgrade refuses before it looks anything up. If the
// guard ever misses, this release version goes on to the real upgrade, so HOME
// is a temp dir: a regression fails here without touching ~/.shipyard.
func TestUpgradeRefusesWhenSelfUpdateOff(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
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
