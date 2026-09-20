package logging

import (
	"io"
	"log"
	"os"

	"github.com/spf13/viper"
)

// Register initializes a log writer that writes either to stderr or nowhere,
// depending on whether the verbose output is enabled.
//
// Logs go to stderr, never stdout: `shipyard mcp serve` speaks JSON-RPC over
// stdout, and a single log line on that stream makes the client's parser fail.
func Register() {
	var logWriter io.Writer
	if viper.GetBool("verbose") {
		logWriter = os.Stderr
	} else {
		logWriter = io.Discard
	}

	log.SetOutput(logWriter)
	log.SetPrefix("SHIPYARD CLI\t")
	log.SetFlags(0)
}
