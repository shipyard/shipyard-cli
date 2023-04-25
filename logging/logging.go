package logging

import (
	"io"
	"log"
	"os"

	"github.com/spf13/viper"
)

// Register initializes a log writer that writes either to stdout or nowhere,
// depending on whether the verbose output is enabled.
func Register() {
	var logWriter io.Writer
	if viper.GetBool("verbose") {
		logWriter = os.Stdout
	} else {
		logWriter = io.Discard
	}

	log.SetOutput(logWriter)
	log.SetPrefix("SHIPYARD CLI\t")
	log.SetFlags(0)
}
