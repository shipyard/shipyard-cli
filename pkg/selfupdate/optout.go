package selfupdate

import "os"

// NoCheckEnv turns the update check off.
const NoCheckEnv = "SHIPYARD_NO_UPDATE_CHECK"

// ChecksOff reports whether update checks are turned off: by NoCheckEnv, by
// running in CI, or by the update_check setting, which the caller passes as
// enabled. The terminal notice and the MCP server's notice both honor it.
func ChecksOff(enabled bool) bool {
	return os.Getenv(NoCheckEnv) != "" || os.Getenv("CI") != "" || !enabled
}
