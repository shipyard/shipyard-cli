package logging

import (
	"log"
	"os"
	"testing"

	"github.com/spf13/viper"
)

// Verbose logs must land on stderr. `shipyard mcp serve` speaks JSON-RPC over
// stdout, so a log line there breaks the client's parser mid-session.
func TestRegisterWritesVerboseLogsToStderr(t *testing.T) {
	original := log.Writer()
	t.Cleanup(func() {
		log.SetOutput(original)
		viper.Set("verbose", false)
	})

	viper.Set("verbose", true)
	Register()

	if log.Writer() != os.Stderr {
		t.Errorf("Expected verbose logs on stderr, got a different writer")
	}
	if log.Writer() == os.Stdout {
		t.Error("Verbose logs on stdout would corrupt the MCP JSON-RPC stream")
	}
}
